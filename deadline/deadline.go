// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package deadline provides deadline timer used to implement
// net.Conn compatible connection
package deadline

import (
	"context"
	"sync"
	"time"
)

type deadlineState uint8

const (
	deadlineStopped deadlineState = iota
	deadlineStarted
	deadlineExceeded
)

var _ context.Context = (*Deadline)(nil)

// Deadline signals updatable deadline timer.
// Also, it implements context.Context.
type Deadline struct {
	mu       sync.RWMutex
	timer    timer
	done     chan struct{}
	deadline time.Time
	state    deadlineState
	pending  uint8
	cbs      map[int]func()
	nextCbID int
}

// New creates new deadline timer.
func New() *Deadline {
	return &Deadline{
		done: make(chan struct{}),
		cbs:  map[int]func(){},
	}
}

func (d *Deadline) timeout() {
	d.mu.Lock()
	if d.pending--; d.pending != 0 || d.state != deadlineStarted {
		d.mu.Unlock()

		return
	}

	d.state = deadlineExceeded
	done := d.done
	for _, cb := range d.cbs {
		go cb()
	}
	clear(d.cbs)
	d.mu.Unlock()

	close(done)
}

// AfterFunc attaches a function to the deadline.
// The functions will be triggered on deadline exceeded.
// If the deadline is reset, the functions are skipped.
// Attached functions are only triggered once.
func (d *Deadline) AfterFunc(cb func()) func() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cbs[d.nextCbID] = cb
	usedID := d.nextCbID
	cancel := func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, has := d.cbs[usedID]; has {
			delete(d.cbs, usedID)

			return true
		}

		return false
	}
	d.nextCbID++

	return cancel
}

// Set new deadline. Zero value means no deadline.
func (d *Deadline) Set(setTo time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == deadlineStarted && d.timer.Stop() {
		d.pending--
	}

	d.deadline = setTo
	d.pending++

	if d.state == deadlineExceeded {
		d.done = make(chan struct{})
	}

	if setTo.IsZero() {
		d.pending--
		d.state = deadlineStopped
		clear(d.cbs)

		return
	}

	if dur := time.Until(setTo); dur > 0 {
		d.state = deadlineStarted
		if d.timer == nil {
			d.timer = afterFunc(dur, d.timeout)
		} else {
			d.timer.Reset(dur)
		}

		return
	}

	d.pending--
	d.state = deadlineExceeded
	close(d.done)
	for _, cb := range d.cbs {
		go cb()
	}
	clear(d.cbs)
}

// Done receives deadline signal.
func (d *Deadline) Done() <-chan struct{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.done
}

// Err returns context.DeadlineExceeded if the deadline is exceeded.
// Otherwise, it returns nil.
func (d *Deadline) Err() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.state == deadlineExceeded {
		return context.DeadlineExceeded
	}

	return nil
}

// Deadline returns current deadline.
func (d *Deadline) Deadline() (time.Time, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.deadline.IsZero() {
		return d.deadline, false
	}

	return d.deadline, true
}

// Value returns nil.
func (d *Deadline) Value(any) any {
	return nil
}
