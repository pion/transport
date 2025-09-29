// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"sync"
	"sync/atomic"
	"time"
)

// DelayFilter delays inbound packets by the given delay. Automatically starts
// processing when created and runs until Close() is called.
type DelayFilter struct {
	NIC
	delay atomic.Int64 // atomic field - stores time.Duration as int64
	push  chan struct{}
	queue *chunkQueue
	done  chan struct{}
	wg    sync.WaitGroup
}

type timedChunk struct {
	Chunk
	deadline time.Time
}

// NewDelayFilter creates and starts a new DelayFilter with the given nic and delay.
func NewDelayFilter(nic NIC, delay time.Duration) (*DelayFilter, error) {
	f := &DelayFilter{
		NIC:   nic,
		push:  make(chan struct{}),
		queue: newChunkQueue(0, 0),
		done:  make(chan struct{}),
	}

	f.delay.Store(int64(delay))

	// Start processing automatically
	f.wg.Add(1)
	go f.run()

	return f, nil
}

// SetDelay atomically updates the delay
func (f *DelayFilter) SetDelay(newDelay time.Duration) {
	f.delay.Store(int64(newDelay))
}

func (f *DelayFilter) getDelay() time.Duration {
	return time.Duration(f.delay.Load())
}

func (f *DelayFilter) onInboundChunk(c Chunk) {
	f.queue.push(timedChunk{
		Chunk:    c,
		deadline: time.Now().Add(f.getDelay()),
	})
	f.push <- struct{}{}
}

// run processes the delayed packets queue until Close() is called.
func (f *DelayFilter) run() {
	defer f.wg.Done()

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-f.done:
			// Drain remaining packets immediately on shutdown
			for {
				next, ok := f.queue.pop()
				if !ok {
					break
				}
				if timedChunk, ok := next.(timedChunk); ok {
					f.NIC.onInboundChunk(timedChunk.Chunk)
				}
			}
			return

		case <-f.push:
			// New packet arrived, update timer for next deadline
			next := f.queue.peek()
			if next != nil {
				if timedChunk, ok := next.(timedChunk); ok {
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(time.Until(timedChunk.deadline))
				}
			}

		case now := <-timer.C:
			// Process all ready packets
			for {
				next := f.queue.peek()
				if next == nil {
					break
				}

				if timedChunk, ok := next.(timedChunk); ok && timedChunk.deadline.Before(now) {
					_, _ = f.queue.pop() // We already have the item from peek()
					f.NIC.onInboundChunk(timedChunk.Chunk)
				} else {
					break
				}
			}

			// Schedule timer for next packet
			next := f.queue.peek()
			if next == nil {
				timer.Reset(time.Minute) // Long timeout when queue is empty
			} else if timedChunk, ok := next.(timedChunk); ok {
				timer.Reset(time.Until(timedChunk.deadline))
			}
		}
	}
}

// Close stops the DelayFilter and waits for graceful shutdown.
func (f *DelayFilter) Close() error {
	close(f.done)
	f.wg.Wait()
	return nil
}
