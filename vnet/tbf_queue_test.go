// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTBFQueue(t *testing.T) {
	t.Run("enqueue-dequeue", func(t *testing.T) {
		q := NewTBFQueue(1_000_000, 10_000, 15*1500)
		chunk := &chunkUDP{
			userData: make([]byte, 1300),
		}
		q.push(chunk)
		res := q.pop()
		assert.Equal(t, chunk, res)
	})
	t.Run("drop-packets", func(t *testing.T) {
		q := NewTBFQueue(1_000_000, 10_000, 15*1500)
		chunk := &chunkUDP{
			userData: make([]byte, 1500),
		}
		for i := 0; i < 20; i++ {
			q.push(chunk)
		}
		assert.Len(t, q.chunks, 15)
	})
	t.Run("burst", func(t *testing.T) {
		queue := NewTBFQueue(150_000, 10*1500, 15*1500)
		chunk := &chunkUDP{
			userData: make([]byte, 1500),
		}
		for i := 0; i < 15; i++ {
			queue.push(chunk)
		}
		// queue size is 15, burst is 10 so we should be allowed to burst 10
		// packets immediately
		for i := 0; i < 10; i++ {
			assert.Equal(t, chunk, queue.pop())
		}
		// But no more than 10
		assert.Nil(t, queue.pop())
	})
	t.Run("rate", func(t *testing.T) {
		queue := NewTBFQueue(8*15_000, 1500, 30_000)
		chunk := &chunkUDP{
			userData: make([]byte, 1000),
		}
		for i := 0; i < 30; i++ {
			queue.push(chunk)
		}
		now := time.Now()
		received := 0
		// dequeue for one second
		for time.Since(now) < time.Second {
			res := queue.pop()
			if res == nil {
				next := queue.next()
				if next.IsZero() {
					continue
				}
				time.Sleep(time.Until(queue.next()))

				continue
			}
			assert.NotNil(t, res, "received nil chunk after receiving %v bytes", received)
			received += len(res.UserData())
		}
		// Initial burst of 1000+1s*15_000 byte/s = 16_000 bytes
		assert.Equal(t, 16_000, received)
	})
}
