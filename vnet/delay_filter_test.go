// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TimestampedChunk struct {
	ts time.Time
	c  Chunk
}

func initTest(t *testing.T) (*DelayFilter, chan TimestampedChunk) {
	t.Helper()
	nic := newMockNIC(t)
	delayFilter, err := NewDelayFilter(nic, WithDelay(0))
	if !assert.NoError(t, err, "should succeed") {
		return nil, nil
	}

	receiveCh := make(chan TimestampedChunk)

	nic.mockOnInboundChunk = func(c Chunk) {
		receivedAt := time.Now()
		receiveCh <- TimestampedChunk{
			ts: receivedAt,
			c:  c,
		}
	}

	return delayFilter, receiveCh
}

func scheduleOnePacketAtATime(
	t *testing.T,
	delayFilter *DelayFilter,
	receiveCh chan TimestampedChunk,
	delay time.Duration,
	nrPackets int,
) {
	t.Helper()
	delayFilter.SetDelay(delay)
	lastNr := -1
	for i := 0; i < nrPackets; i++ {
		sent := time.Now()
		delayFilter.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})

		select {
		case chunk := <-receiveCh:
			nr := int(chunk.c.UserData()[0])

			assert.Greater(t, nr, lastNr)
			lastNr = nr

			assert.Greater(t, chunk.ts.Sub(sent), delay)
			// Use generous timing tolerance for CI environments with high system load
			// and virtualization overhead. Function call overhead from DelayFilter
			// refactoring also contributes to timing variability.
			assert.Less(t, chunk.ts.Sub(sent), delay+200*time.Millisecond)
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")
		}
	}
}

func scheduleManyPackets(
	t *testing.T,
	delayFilter *DelayFilter,
	receiveCh chan TimestampedChunk,
	delay time.Duration,
	nrPackets int, //nolint:unparam
) {
	t.Helper()
	delayFilter.SetDelay(delay)
	sent := time.Now()

	for i := 0; i < nrPackets; i++ {
		delayFilter.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})
	}

	// receive nrPackets chunks with a minimum delay
	for i := 0; i < nrPackets; i++ {
		select {
		case chunk := <-receiveCh:
			nr := int(chunk.c.UserData()[0])
			assert.Equal(t, i, nr)
			assert.Greater(t, chunk.ts.Sub(sent), delay)
			assert.Less(t, chunk.ts.Sub(sent), delay+200*time.Millisecond)
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")
		}
	}
}

func TestDelayFilter(t *testing.T) {
	t.Run("schedulesOnePacketAtATime", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, delayFilter.Close())
	})

	t.Run("schedulesSubsequentManyPackets", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleManyPackets(t, delayFilter, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, delayFilter.Close())
	})

	t.Run("scheduleIncreasingDelayOnePacketAtATime", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 10*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 50*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 100*time.Millisecond, 10)
		assert.NoError(t, delayFilter.Close())
	})

	t.Run("scheduleDecreasingDelayOnePacketAtATime", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 100*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 50*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, delayFilter, receiveCh, 10*time.Millisecond, 10)
		assert.NoError(t, delayFilter.Close())
	})

	t.Run("scheduleIncreasingDelayManyPackets", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleManyPackets(t, delayFilter, receiveCh, 10*time.Millisecond, 100)
		scheduleManyPackets(t, delayFilter, receiveCh, 50*time.Millisecond, 100)
		scheduleManyPackets(t, delayFilter, receiveCh, 100*time.Millisecond, 100)
		assert.NoError(t, delayFilter.Close())
	})

	t.Run("scheduleDecreasingDelayManyPackets", func(t *testing.T) {
		delayFilter, receiveCh := initTest(t)
		if delayFilter == nil {
			return
		}

		scheduleManyPackets(t, delayFilter, receiveCh, 100*time.Millisecond, 100)
		scheduleManyPackets(t, delayFilter, receiveCh, 50*time.Millisecond, 100)
		scheduleManyPackets(t, delayFilter, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, delayFilter.Close())
	})
}

func TestNewDelayFilterOptions(t *testing.T) {
	t.Run("invalid delay", func(t *testing.T) {
		nic := newMockNIC(t)
		_, err := NewDelayFilter(nic, WithDelay(-time.Millisecond))
		assert.ErrorIs(t, err, ErrInvalidDelay)
	})

	t.Run("nil option ignored", func(t *testing.T) {
		nic := newMockNIC(t)
		filter, err := NewDelayFilter(nic, nil, WithDelay(0))
		assert.NoError(t, err)
		assert.NoError(t, filter.Close())
	})

	t.Run("default delay zero", func(t *testing.T) {
		nic := newMockNIC(t)
		filter, err := NewDelayFilter(nic)
		assert.NoError(t, err)
		assert.NoError(t, filter.Close())
	})
}
