// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package udp

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

var errBatchCloseTest = errors.New("batch close test")

type shutdownBatchConn struct {
	entered  chan struct{}
	released chan struct{}
	once     sync.Once
	closeErr error
}

func (c *shutdownBatchConn) WriteBatch(messages []ipv4.Message, _ int) (int, error) {
	if c.entered == nil {
		return len(messages), nil
	}
	close(c.entered)
	<-c.released

	return 0, net.ErrClosed
}

func (c *shutdownBatchConn) ReadBatch([]ipv4.Message, int) (int, error) {
	return 0, net.ErrClosed
}

func (c *shutdownBatchConn) Close() error {
	c.once.Do(func() { close(c.released) })

	return c.closeErr
}

func TestBatchConnWriteAfterClose(t *testing.T) {
	batch := &shutdownBatchConn{released: make(chan struct{}), closeErr: errBatchCloseTest}
	conn := &BatchConn{batchConn: batch, batchPingPong: newPingPong(4, time.Millisecond, batch)}
	require.ErrorIs(t, conn.Close(), errBatchCloseTest)
	for _, payload := range [][]byte{nil, {1}, {2, 3}} {
		n, err := conn.WriteTo(payload, nil)
		assert.Zero(t, n)
		assert.ErrorIs(t, err, net.ErrClosed)
	}
}

func TestBatchConnCloseUnblocksBatchWrite(t *testing.T) {
	batch := &shutdownBatchConn{
		entered: make(chan struct{}), released: make(chan struct{}), closeErr: errBatchCloseTest,
	}
	conn := &BatchConn{batchConn: batch, batchPingPong: newPingPong(1, time.Millisecond, batch)}
	t.Cleanup(func() {
		require.ErrorIs(t, batch.Close(), errBatchCloseTest)
		require.ErrorIs(t, conn.Close(), errBatchCloseTest)
	})
	n, err := conn.WriteTo([]byte{1}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	select {
	case <-batch.entered:
	case <-time.After(time.Second):
		require.FailNow(t, "batch write never started")
	}
	closed := make(chan error, 1)
	go func() { closed <- conn.Close() }()
	select {
	case err = <-closed:
		assert.ErrorIs(t, err, errBatchCloseTest)
	case <-time.After(100 * time.Millisecond):
		require.ErrorIs(t, batch.Close(), errBatchCloseTest)
		require.ErrorIs(t, <-closed, errBatchCloseTest)
		require.FailNow(t, "Close waited for WriteBatch before closing the underlying socket")
	}
}

func TestBatchConnCloseWaitingWriter(t *testing.T) {
	batch := &shutdownBatchConn{
		entered: make(chan struct{}), released: make(chan struct{}),
	}
	queue := newPingPong(1, time.Millisecond, batch)
	conn := &BatchConn{batchConn: batch, batchPingPong: queue}
	t.Cleanup(func() {
		require.NoError(t, batch.Close())
		require.NoError(t, conn.Close())
	})
	_, err := conn.WriteTo([]byte{1}, nil)
	require.NoError(t, err)
	select {
	case <-batch.entered:
	case <-time.After(time.Second):
		require.FailNow(t, "batch write never started")
	}
	_, err = conn.WriteTo([]byte{2}, nil)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		n, writeErr := conn.WriteTo([]byte{1}, nil)
		assert.Zero(t, n)
		result <- writeErr
	}()
	// Synchronize with the writer's wait before closing the connection.
	select {
	case queue.flushCycleDone <- struct{}{}:
	case <-time.After(time.Second):
		require.FailNow(t, "writer never waited for a flush")
	}
	closed := make(chan error, 1)
	go func() { closed <- conn.Close() }()
	select {
	case err = <-closed:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		require.NoError(t, batch.Close())
		require.NoError(t, <-closed)
		assert.Fail(t, "Close waited for WriteBatch before closing the underlying socket")
	}
	select {
	case err := <-result:
		assert.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(time.Second):
		require.FailNow(t, "waiting writer did not return after Close")
	}
}
