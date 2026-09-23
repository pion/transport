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
	"sync/atomic"
	"time"

	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
)

// ErrBusy indicates that another Check is active.
var ErrBusy = errors.New("network change detector is busy")

var (
	errInvalidPlatformTimeout = errors.New("platform timeout must not be negative")
	errInvalidPollInterval    = errors.New("poll interval must be positive")
)

const defaultPollInterval = time.Second

type notificationSource interface {
	drain() (bool, error)
	wait(context.Context) error
	close() error
}

// Detector implements transport.Net using the latest system interfaces and addresses.
// Check waits for changes and refreshes interfaces internally. Network methods
// can be called concurrently with Check. A Detector must not be copied.
type Detector struct {
	mu              sync.Mutex
	checking        bool
	closed          bool
	state           stateDetector
	source          notificationSource
	enumerate       func() ([]interfaceState, error)
	pending         bool
	initial         []Change // nil after the initial result has been delivered.
	network         atomic.Pointer[stdnet.Net]
	platformTimeout time.Duration
	pollInterval    time.Duration
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
	detector := &Detector{pollInterval: defaultPollInterval}
	detector.enumerate = func() ([]interfaceState, error) {
		network := &stdnet.Net{}
		state, err := enumerateInterfaces(network)
		if err == nil {
			detector.network.Store(network)
		}

		return state, err
	}
	for _, option := range options {
		if option != nil {
			option(detector)
		}
	}
	if detector.platformTimeout < 0 {
		return nil, errInvalidPlatformTimeout
	}
	if detector.pollInterval <= 0 {
		return nil, errInvalidPollInterval
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

// WithInterfaceFilter reports changes only for interfaces for which filter returns
// true. It does not filter the interfaces exposed through transport.Net.
func WithInterfaceFilter(filter func(string) bool) Option {
	return func(detector *Detector) {
		detector.state.interfaceFilter = filter
	}
}

// WithPlatformTimeout limits how long Check waits for a native notification before
// refreshing interfaces anyway. If nothing changed, Check continues waiting.
// Zero (the default) disables the timeout.
func WithPlatformTimeout(timeout time.Duration) Option {
	return func(detector *Detector) {
		detector.platformTimeout = timeout
	}
}

// WithPollInterval sets the interval between polls when native notifications are
// unavailable. It defaults to one second and must be positive.
func WithPollInterval(interval time.Duration) Option {
	return func(detector *Detector) {
		detector.pollInterval = interval
	}
}

func (d *Detector) start() error {
	if d.pollInterval == 0 {
		d.pollInterval = defaultPollInterval
	}
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

// Check updates the internal network snapshot and waits until an included
// interface changes. The first call reports the initial
// matching interfaces as Added, returning immediately even if none match.
// Subsequent successful calls contain at least one affected interface.
// Cancellation returns context.Cause(ctx), including custom cancellation causes.
// Check returns ErrBusy while another Check is active.
func (d *Detector) Check(ctx context.Context) ([]Change, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()

		return nil, os.ErrClosed
	}
	if d.checking {
		d.mu.Unlock()

		return nil, ErrBusy
	}
	if err := context.Cause(ctx); err != nil {
		d.mu.Unlock()

		return nil, err
	}
	d.checking = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.checking = false
		d.mu.Unlock()
	}()

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
	if d.source == nil {
		timer := time.NewTimer(d.pollInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-timer.C:
			return nil
		}
	}
	d.pending = true
	if d.platformTimeout <= 0 {
		return d.source.wait(ctx)
	}
	waitCtx, cancel := context.WithTimeout(ctx, d.platformTimeout)
	defer cancel()
	err := d.source.wait(waitCtx)
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if errors.Is(err, context.DeadlineExceeded) && waitCtx.Err() != nil {
		return nil
	}

	return err
}

// Close releases the platform backend.
// It returns ErrBusy while Check is active.
func (d *Detector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return os.ErrClosed
	}
	if d.checking {
		return ErrBusy
	}
	d.closed = true
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

func enumerateInterfaces(network *stdnet.Net) ([]interfaceState, error) {
	if err := network.UpdateInterfaces(); err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}
	interfaces, _ := network.Interfaces()

	return interfaceStates(interfaces)
}

func interfaceStates(interfaces []*transport.Interface) ([]interfaceState, error) {
	state := make([]interfaceState, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if errors.Is(err, transport.ErrNoAddressAssigned) {
			addresses = nil
		} else if err != nil {
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
