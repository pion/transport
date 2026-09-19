// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package netchange detects changes to system interfaces and IP addresses.
// Check waits for a meaningful change or context cancellation.
package netchange

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/wlynxg/anet"
)

// used if OS detection is not supported.
var pollTimerPool sync.Pool //nolint:gochecknoglobals

type notificationSource interface {
	drain() (bool, error)
	wait(context.Context) error
	close() error
}

// Detector checks system interfaces for changes.
type Detector struct {
	state     stateDetector
	source    notificationSource
	enumerate func() ([]interfaceState, error)
	pending   bool
	initial   []Change // nil after the initial result has been delivered.
}

// Option configures a Detector during construction.
type Option func(*Detector)

// ChangeType describes how an interface changed.
type ChangeType uint8

const (
	Added   ChangeType = iota + 1 // Added indicates an interface entered the monitored list.
	Removed                       // Removed indicates an interface left the monitored list.
	Changed                       // Changed indicates an interface's name, flags, or addresses changed.
)

// Change identifies one affected interface change.
type Change struct {
	Interface string
	Type      ChangeType
}

// NewDetector opens the platform backend and captures the initial interfaces.
func NewDetector(options ...Option) (*Detector, error) {
	detector := &Detector{enumerate: enumerateInterfaces}
	for _, option := range options {
		if option != nil {
			option(detector)
		}
	}
	var err error
	detector.source, err = openSocket()
	if err != nil {
		return nil, err
	}
	if err := detector.start(); err != nil {
		return nil, err
	}

	return detector, nil
}

// WithInterfaceFilter includes only interfaces for which filter returns true.
func WithInterfaceFilter(filter func(string) bool) Option {
	return func(detector *Detector) {
		detector.state.interfaceFilter = filter
	}
}

func (d *Detector) start() error {
	interfaces, err := d.enumerate()
	if err != nil {
		if d.source != nil {
			err = errors.Join(err, d.source.close())
		}

		return err
	}
	d.initial = d.state.update(interfaces)

	return nil
}

// Check waits until an included interface changes, the first call reports the initial
// matching interfaces as Added, returning immediately even if none match.
// Subsequent successful calls contain at least one affected interface.
// Cancellation returns context.Cause(ctx), including custom cancellation causes.
func (d *Detector) Check(ctx context.Context) ([]Change, error) {
	if d.state.last == nil {
		return nil, os.ErrClosed
	}
	for {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if d.initial != nil {
			changes := d.initial
			d.initial = nil

			return changes, nil
		}
		change, err := d.refresh(ctx)
		if err != nil || len(change) != 0 {
			return change, err
		}
		if err := d.wait(ctx); err != nil {
			return nil, err
		}
	}
}

func (d *Detector) refresh(ctx context.Context) ([]Change, error) {
	if d.source != nil {
		notified, err := d.source.drain()
		d.pending = d.pending || notified || err != nil
		if err != nil {
			return nil, err
		}
		if !d.pending {
			return nil, nil
		}
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	interfaces, err := d.enumerate()
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if err != nil {
		return nil, err
	}
	d.pending = false

	return d.state.update(interfaces), nil
}

func (d *Detector) wait(ctx context.Context) error {
	if d.source != nil {
		d.pending = true

		return d.source.wait(ctx)
	}
	timer, ok := pollTimerPool.Get().(*time.Timer)
	if !ok {
		timer = time.NewTimer(time.Second)
	} else {
		timer.Reset(time.Second)
	}
	defer func() {
		timer.Stop()
		select {
		case <-timer.C:
		default:
		}
		pollTimerPool.Put(timer)
	}()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

// Close releases the platform backend.
func (d *Detector) Close() error {
	if d.state.last == nil {
		return os.ErrClosed
	}
	d.state.last = nil
	if d.source != nil {
		return d.source.close()
	}

	return nil
}

type interfaceState struct {
	Index int
	Name  string
	Flags net.Flags
	Addrs map[netip.Prefix]struct{}
}

type stateDetector struct {
	last            []interfaceState // nil before initialization or after Close.
	interfaceFilter func(string) bool
}

func (d *stateDetector) update(next []interfaceState) []Change {
	next = normalize(next, d.interfaceFilter)
	changes := make([]Change, 0)
	previous := make(map[int]interfaceState, len(d.last))
	for _, iface := range d.last {
		previous[iface.Index] = iface
	}
	for _, iface := range next {
		before, exists := previous[iface.Index]
		switch {
		case !exists:
			changes = append(changes, Change{Interface: iface.Name, Type: Added})
		case before.Name != iface.Name || before.Flags != iface.Flags || !maps.Equal(before.Addrs, iface.Addrs):
			changes = append(changes, Change{Interface: iface.Name, Type: Changed})
		}
		delete(previous, iface.Index)
	}
	for _, iface := range d.last {
		if _, exists := previous[iface.Index]; exists {
			changes = append(changes, Change{Interface: iface.Name, Type: Removed})
		}
	}
	d.last = next

	return changes
}

func normalize(interfaces []interfaceState, filter func(string) bool) []interfaceState {
	state := make([]interfaceState, 0, len(interfaces))
	for _, iface := range interfaces {
		if filter != nil && !filter(iface.Name) {
			continue
		}
		iface.Addrs = maps.Clone(iface.Addrs)
		state = append(state, iface)
	}
	slices.SortFunc(state, func(left, right interfaceState) int {
		return cmp.Compare(left.Index, right.Index)
	})

	return state
}

func enumerateInterfaces() ([]interfaceState, error) {
	interfaces, err := anet.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}
	state := make([]interfaceState, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses, err := anet.InterfaceAddrsByInterface(&iface)
		if err != nil {
			return nil, fmt.Errorf("enumerate addresses for %s: %w", iface.Name, err)
		}
		next := interfaceState{
			Index: iface.Index, Name: iface.Name, Flags: iface.Flags,
			Addrs: make(map[netip.Prefix]struct{}, len(addresses)),
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil {
				return nil, fmt.Errorf("parse address for %s: %w", iface.Name, err)
			}
			next.Addrs[prefix] = struct{}{}
		}
		state = append(state, next)
	}

	return state, nil
}
