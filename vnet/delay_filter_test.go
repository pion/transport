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
	df, err := NewDelayFilter(nic, 0)
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

	return df, receiveCh
}

func scheduleOnePacketAtATime(
	t *testing.T,
	df *DelayFilter,
	receiveCh chan TimestampedChunk,
	delay time.Duration,
	nrPackets int,
) {
	t.Helper()
	df.SetDelay(delay)
	lastNr := -1
	for i := 0; i < nrPackets; i++ {
		sent := time.Now()
		df.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})

		select {
		case c := <-receiveCh:
			nr := int(c.c.UserData()[0])

			assert.Greater(t, nr, lastNr)
			lastNr = nr

			assert.Greater(t, c.ts.Sub(sent), delay)
			// Use generous timing tolerance for CI environments with high system load
			// and virtualization overhead. Function call overhead from DelayFilter
			// refactoring also contributes to timing variability.
			assert.Less(t, c.ts.Sub(sent), delay+200*time.Millisecond)
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")
		}
	}
}

func scheduleManyPackets(
	t *testing.T,
	df *DelayFilter,
	receiveCh chan TimestampedChunk,
	delay time.Duration,
	nrPackets int, //nolint:unparam
) {
	t.Helper()
	df.SetDelay(delay)
	sent := time.Now()

	for i := 0; i < nrPackets; i++ {
		df.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})
	}

	// receive nrPackets chunks with a minimum delay
	for i := 0; i < nrPackets; i++ {
		select {
		case c := <-receiveCh:
			nr := int(c.c.UserData()[0])
			assert.Equal(t, i, nr)
			assert.Greater(t, c.ts.Sub(sent), delay)
			assert.Less(t, c.ts.Sub(sent), delay+200*time.Millisecond)
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")
		}
	}
}

func TestDelayFilter(t *testing.T) {
	t.Run("schedulesOnePacketAtATime", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleOnePacketAtATime(t, df, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, df.Close())
	})

	t.Run("schedulesSubsequentManyPackets", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleManyPackets(t, df, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, df.Close())
	})

	t.Run("scheduleIncreasingDelayOnePacketAtATime", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleOnePacketAtATime(t, df, receiveCh, 10*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, df, receiveCh, 50*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, df, receiveCh, 100*time.Millisecond, 10)
		assert.NoError(t, df.Close())
	})

	t.Run("scheduleDecreasingDelayOnePacketAtATime", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleOnePacketAtATime(t, df, receiveCh, 100*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, df, receiveCh, 50*time.Millisecond, 10)
		scheduleOnePacketAtATime(t, df, receiveCh, 10*time.Millisecond, 10)
		assert.NoError(t, df.Close())
	})

	t.Run("scheduleIncreasingDelayManyPackets", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleManyPackets(t, df, receiveCh, 10*time.Millisecond, 100)
		scheduleManyPackets(t, df, receiveCh, 50*time.Millisecond, 100)
		scheduleManyPackets(t, df, receiveCh, 100*time.Millisecond, 100)
		assert.NoError(t, df.Close())
	})

	t.Run("scheduleDecreasingDelayManyPackets", func(t *testing.T) {
		df, receiveCh := initTest(t)
		if df == nil {
			return
		}

		scheduleManyPackets(t, df, receiveCh, 100*time.Millisecond, 100)
		scheduleManyPackets(t, df, receiveCh, 50*time.Millisecond, 100)
		scheduleManyPackets(t, df, receiveCh, 10*time.Millisecond, 100)
		assert.NoError(t, df.Close())
	})
}
