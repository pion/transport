// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockDiscipline struct {
	mockPush  func(Chunk)
	mockPop   func() Chunk
	mockEmpty func() bool
	mockNext  func() time.Time
}

// empty implements Discipline.
func (m *mockDiscipline) empty() bool {
	return m.mockEmpty()
}

// next implements Discipline.
func (m *mockDiscipline) next() time.Time {
	return m.mockNext()
}

// pop implements Discipline.
func (m *mockDiscipline) pop() Chunk {
	return m.mockPop()
}

// push implements Discipline.
func (m *mockDiscipline) push(c Chunk) {
	m.mockPush(c)
}

func newMockDiscipline(t *testing.T) *mockDiscipline {
	t.Helper()

	return &mockDiscipline{
		mockPush: func(Chunk) {
			assert.Fail(t, "unexpected call to push")
		},
		mockPop: func() Chunk {
			assert.Fail(t, "unexpected call to pop")

			return nil
		},
		mockEmpty: func() bool {
			assert.Fail(t, "unexpected call to empty")

			return false
		},
		mockNext: func() time.Time {
			assert.Fail(t, "unexpected call to next")

			return time.Time{}
		},
	}
}

func TestQueue(t *testing.T) {
	t.Run("enqueue-chunk", func(t *testing.T) {
		mnic := newMockNIC(t)
		md := newMockDiscipline(t)
		pushCh := make(chan struct{})
		chunk := &chunkUDP{
			userData: make([]byte, 1300),
		}
		md.mockPush = func(c Chunk) {
			assert.Equal(t, chunk, c)
			close(pushCh)
		}
		md.mockEmpty = func() bool {
			return true
		}
		q, err := NewQueue(mnic, md)
		assert.NoError(t, err)

		q.onInboundChunk(chunk)

		select {
		case <-pushCh:
		case <-time.After(10 * time.Millisecond):
			assert.Fail(t, "timeout before chunk was pushed")
		}
		assert.NoError(t, q.Close())
	})
	t.Run("dequeue-chunk", func(t *testing.T) {
		mnic := newMockNIC(t)

		data := []Chunk{
			&chunkUDP{
				userData: make([]byte, 1300),
			},
		}
		pushCh := make(chan struct{})
		mnic.mockOnInboundChunk = func(c Chunk) {
			close(pushCh)
		}
		md := newMockDiscipline(t)

		md.mockEmpty = func() bool {
			return len(data) == 0
		}
		md.mockPop = func() Chunk {
			var next Chunk
			next, data = data[0], data[1:]

			return next
		}
		// return false the first time and only dequeue after 5ms
		nextCalled := false
		md.mockNext = func() time.Time {
			if nextCalled {
				return time.Time{}
			}
			nextCalled = true

			return time.Now().Add(5 * time.Millisecond)
		}

		q, err := NewQueue(mnic, md)
		assert.NoError(t, err)

		select {
		case <-pushCh:
		case <-time.After(10 * time.Millisecond):
			assert.Fail(t, "timeout before chunk was pushed")
		}

		assert.NoError(t, q.Close())
	})
}
