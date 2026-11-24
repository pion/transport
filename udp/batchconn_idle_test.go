// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package udp

// The batch flusher sleeps while the write batch is empty (see
// pingPong.flusher). These tests drive pingPong with a counting batch conn,
// so they run on every platform (the socket tests need linux).

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/transport/v5/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/ipv4"
)

type countingBatchConn struct {
	mu     sync.Mutex
	writes int // WriteBatch calls
	msgs   int // messages written
	at     []time.Time
}

func (c *countingBatchConn) WriteBatch(ms []ipv4.Message, _ int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	c.msgs += len(ms)
	c.at = append(c.at, time.Now())

	return len(ms), nil
}

func (c *countingBatchConn) ReadBatch([]ipv4.Message, int) (int, error) { return 0, io.EOF }

func (c *countingBatchConn) Close() error { return nil }

func (c *countingBatchConn) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.writes, c.msgs
}

// wrote reports whether n WriteBatch calls happened.
func (c *countingBatchConn) wrote(n int) func() bool {
	return func() bool {
		writes, _ := c.counts()

		return writes == n
	}
}

// sent reports whether n messages were written.
func (c *countingBatchConn) sent(n int) func() bool {
	return func() bool {
		_, msgs := c.counts()

		return msgs == n
	}
}

// waitFor polls cond every millisecond up to limit.
func waitFor(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}

	return cond()
}

func wakeups(pp *pingPong) int32 { return atomic.LoadInt32(&pp.wakeups) }

// With nothing to send the flusher never wakes: the former interval/2 ticker
// would have woken it about 200 times in this test.
func TestPingPong_IdleDoesNotWake(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	conn := &countingBatchConn{}
	pp := newPingPong(128, 2*time.Millisecond, conn)
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, wakeups(pp), "idle flusher woke up")
	writes, _ := conn.counts()
	assert.Zero(t, writes)
	pp.Close()
}

// One message on an idle connection leaves within the interval, and the
// flusher goes back to sleep afterwards.
func TestPingPong_FirstMessageLeavesWithinInterval(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	const interval = 20 * time.Millisecond
	conn := &countingBatchConn{}
	pp := newPingPong(128, interval, conn)
	defer pp.Close()

	t0 := time.Now()
	assert.Equal(t, 5, pp.EnqueueMessage([]byte("hello"), nil))
	assert.True(t, waitFor(time.Second, conn.wrote(1)), "message never flushed")
	waited := conn.at[0].Sub(t0)
	// the OS timer may add a few milliseconds; never a second interval
	assert.Less(t, waited, interval+15*time.Millisecond, "waited %v for a %v interval", waited, interval)
	assert.GreaterOrEqual(t, waited, interval/2, "flushed before the batch could fill: %v", waited)
	_, msgs := conn.counts()
	assert.Equal(t, 1, msgs)

	// idle again: no further wake-up
	after := wakeups(pp)
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, after, wakeups(pp), "flusher kept waking after the batch emptied")
	writes, _ := conn.counts()
	assert.Equal(t, 1, writes)
}

// A full batch does not wait for the timer.
func TestPingPong_FullBatchFlushesAtOnce(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	conn := &countingBatchConn{}
	pp := newPingPong(4, time.Minute, conn)
	defer pp.Close()

	for range 4 {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte("x"), nil))
	}
	assert.True(t, waitFor(200*time.Millisecond, conn.wrote(1)), "full batch not flushed")
	_, msgs := conn.counts()
	assert.Equal(t, 4, msgs)
}

// Second batch filling while the first is still being written (the writer
// is slow): its flush is not lost and nobody waits for the minute-long timer.
func TestPingPong_FullBatchDuringFlushIsNotLost(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	slow := &slowBatchConn{hold: make(chan struct{})}
	pp := newPingPong(2, time.Minute, slow)
	defer pp.Close()

	pp.EnqueueMessage([]byte("a"), nil)
	pp.EnqueueMessage([]byte("b"), nil) // full: flush 1 starts and blocks in WriteBatch
	assert.True(t, waitFor(200*time.Millisecond, func() bool { return atomic.LoadInt32(&slow.inWrite) == 1 }))
	pp.EnqueueMessage([]byte("c"), nil)
	pp.EnqueueMessage([]byte("d"), nil) // full again while the flusher is busy
	close(slow.hold)
	assert.True(t, waitFor(500*time.Millisecond, func() bool { return atomic.LoadInt32(&slow.msgs) == 4 }),
		"second full batch waited for the timer")
}

type slowBatchConn struct {
	hold    chan struct{}
	inWrite int32
	msgs    int32
}

func (c *slowBatchConn) WriteBatch(ms []ipv4.Message, _ int) (int, error) {
	atomic.StoreInt32(&c.inWrite, 1)
	<-c.hold
	atomic.AddInt32(&c.msgs, int32(len(ms))) //nolint:gosec

	return len(ms), nil
}

func (c *slowBatchConn) ReadBatch([]ipv4.Message, int) (int, error) { return 0, io.EOF }

func (c *slowBatchConn) Close() error { return nil }

// A steady trickle (far below the batch size) is paced about one flush per
// interval, every message is delivered, and no message waits beyond the
// interval plus timer slack.
func TestPingPong_TrickleIsPacedByTheInterval(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	const interval = 10 * time.Millisecond
	conn := &countingBatchConn{}
	pp := newPingPong(128, interval, conn)
	defer pp.Close()

	sent := 0
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte("x"), nil))
		sent++
		time.Sleep(time.Millisecond)
	}
	assert.True(t, waitFor(time.Second, conn.sent(sent)), "messages lost")
	writes, _ := conn.counts()
	// ~20 flushes for 200 ms at 10 ms; never one per message, never one per 2 ms tick
	assert.GreaterOrEqual(t, writes, 5)
	assert.LessOrEqual(t, writes, 40)
	conn.mu.Lock()
	defer conn.mu.Unlock()
	for i := 1; i < len(conn.at); i++ {
		gap := conn.at[i].Sub(conn.at[i-1])
		assert.Less(t, gap, 2*interval+15*time.Millisecond, "flush %d came %v after the previous", i, gap)
	}
}

// Close returns while the flusher sleeps and while its timer is armed.
func TestPingPong_CloseWhileIdleAndWhileArmed(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	conn := &countingBatchConn{}
	pp := newPingPong(128, time.Minute, conn)
	done := make(chan struct{})
	go func() { pp.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "Close hung on an idle flusher")
	}

	pp = newPingPong(128, time.Minute, conn)
	pp.EnqueueMessage([]byte("x"), nil) // timer armed for a minute
	assert.True(t, waitFor(200*time.Millisecond, func() bool { return wakeups(pp) == 1 }))
	done = make(chan struct{})
	go func() { pp.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "Close hung on an armed flusher")
	}
}
