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
	}
}

func (d *Deadline) timeout() {
	d.mu.Lock()
	if d.pending--; d.pending != 0 || d.state != deadlineStarted {
		d.mu.Unlock()

		return
	}

	d.fire()
	d.mu.Unlock()
}

// AfterTimeout attaches a callback to the deadline.
// The added callback will be triggered when the deadline is met.
// The callback can be detached via the returned function.
// This function mimics context.AfterFunc behavior.
func (d *Deadline) AfterTimeout(cb func()) func() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state == deadlineExceeded {
		go cb()

		return func() bool { return false }
	}
	d.nextCbID++
	usedID := d.nextCbID
	if d.cbs == nil {
		d.cbs = map[int]func(){}
	}
	d.cbs[d.nextCbID] = cb
	cancel := func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, has := d.cbs[usedID]; has {
			delete(d.cbs, usedID)

			return true
		}

		return false
	}

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
	d.fire()
}

func (d *Deadline) fire() {
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
