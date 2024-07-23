// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
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

// VerifyFunc checks that the deadline handler call is still valid ie.
// SetDeadline has not been called concurrently.
type VerifyFunc func() bool

// Scheduler is a utility for building deadline handlers. Scheduler is not safe
// for concurrent access.
type Scheduler struct {
	f       func(VerifyFunc)
	timer   timer
	state   deadlineState
	pending uint8
}

// NewScheduler creates a Scheduler with the supplied deadline handler function.
// The handler is called once the scheduled deadline is reached. The VerifyFunc
// must be called exactly once and cannot be called concurrently with calls to
// other Scheduler functions.
func NewScheduler(f func(VerifyFunc)) *Scheduler {
	return &Scheduler{
		f: f,
	}
}

func (d *Scheduler) verify() bool {
	if d.pending--; d.pending != 0 || d.state != deadlineStarted {
		return false
	}

	d.state = deadlineExceeded
	return true
}

func (d *Scheduler) timeout() {
	d.f(d.verify)
}

// SetDeadline schedules the function to be called. If t is zero the function
// is unscheduled. The returned reset value is true when the previous deadline
// was exceeded. The returned exceeded value is true when t is in the past.
func (d *Scheduler) SetDeadline(t time.Time) (reset, exceeded bool) {
	reset = d.state == deadlineExceeded

	if d.state == deadlineStarted && d.timer.Stop() {
		d.pending--
	}
	d.pending++

	if t.IsZero() {
		d.pending--
		d.state = deadlineStopped
		return
	}

	if dur := time.Until(t); dur > 0 {
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
	return reset, true
}

// DeadlineExceeded returns true when the last set deadline has passed
func (d *Scheduler) DeadlineExceeded() bool {
	return d.state == deadlineExceeded
}

// Deadline signals updatable deadline timer.
// Also, it implements context.Context.
type Deadline struct {
	mu        sync.RWMutex
	done      chan struct{}
	deadline  time.Time
	scheduler *Scheduler
}

// New creates new deadline timer.
func New() *Deadline {
	d := &Deadline{
		done: make(chan struct{}),
	}
	d.scheduler = NewScheduler(d.handleDeadline)
	return d
}

func (d *Deadline) handleDeadline(verify VerifyFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if verify() {
		close(d.done)
	}
}

// Set new deadline. Zero value means no deadline.
func (d *Deadline) Set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.deadline = t

	reset, exceeded := d.scheduler.SetDeadline(t)
	if reset {
		d.done = make(chan struct{})
	}
	if exceeded {
		close(d.done)
	}
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
	if d.scheduler.DeadlineExceeded() {
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
func (d *Deadline) Value(interface{}) interface{} {
	return nil
}
