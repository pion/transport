// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

package udp

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/transport/v5/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/ipv4"
)

func TestBatchConn_WriteBatchInterval(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	lc := ListenConfig{
		Batch: BatchIOConfig{
			Enable:             true,
			ReadBatchSize:      10,
			WriteBatchSize:     3,
			WriteBatchInterval: 5 * time.Millisecond,
		},
		ReadBufferSize:  64 * 1024,
		WriteBufferSize: 64 * 1024,
	}

	laddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 15678}
	listener, err := lc.Listen("udp", laddr)
	assert.NoError(t, err)

	var serverConnWg sync.WaitGroup
	serverConnWg.Add(1)
	go func() { //nolint:dupl
		var exit int32
		defer func() {
			defer serverConnWg.Done()
			atomic.StoreInt32(&exit, 1)
		}()
		for {
			buf := make([]byte, 1400)
			conn, lerr := listener.Accept()
			if errors.Is(lerr, ErrClosedListener) {
				break
			}
			assert.NoError(t, lerr)
			serverConnWg.Add(1)
			go func() {
				defer func() {
					_ = conn.Close()
					serverConnWg.Done()
				}()
				for atomic.LoadInt32(&exit) != 1 {
					_ = conn.SetReadDeadline(time.Now().Add(time.Second))
					n, rerr := conn.Read(buf)
					if rerr != nil {
						assert.ErrorContains(t, rerr, "timeout")
					} else {
						_, rerr = conn.Write(buf[:n])
						assert.NoError(t, rerr)
					}
				}
			}()
		}
	}()

	raddr, _ := listener.Addr().(*net.UDPAddr)

	// test flush by WriteBatchInterval expired
	readBuf := make([]byte, 1400)
	cli, err := net.DialUDP("udp", nil, raddr)
	assert.NoError(t, err)
	flushStr := "flushbytimer"
	_, err = cli.Write([]byte("flushbytimer"))
	assert.NoError(t, err)
	n, err := cli.Read(readBuf)
	assert.NoError(t, err)
	assert.Equal(t, flushStr, string(readBuf[:n]))

	_ = listener.Close()
	serverConnWg.Wait()
}

func TestBatchConn_WriteBatchSize(t *testing.T) { //nolint:cyclop
	report := test.CheckRoutines(t)
	defer report()

	lc := ListenConfig{
		Batch: BatchIOConfig{
			Enable:             true,
			ReadBatchSize:      10,
			WriteBatchSize:     9,
			WriteBatchInterval: time.Minute,
		},
		ReadBufferSize:  64 * 1024,
		WriteBufferSize: 64 * 1024,
	}

	laddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 15678}
	listener, err := lc.Listen("udp", laddr)
	assert.NoError(t, err)

	var serverConnWg sync.WaitGroup
	serverConnWg.Add(1)
	go func() { //nolint:dupl
		var exit int32
		defer func() {
			defer serverConnWg.Done()
			atomic.StoreInt32(&exit, 1)
		}()
		for {
			buf := make([]byte, 1400)
			conn, lerr := listener.Accept()
			if errors.Is(lerr, ErrClosedListener) {
				break
			}
			assert.NoError(t, lerr)
			serverConnWg.Add(1)
			go func() {
				defer func() {
					_ = conn.Close()
					serverConnWg.Done()
				}()
				for atomic.LoadInt32(&exit) != 1 {
					_ = conn.SetReadDeadline(time.Now().Add(time.Second))
					n, rerr := conn.Read(buf)
					if rerr != nil {
						assert.ErrorContains(t, rerr, "timeout")
					} else {
						_, rerr = conn.Write(buf[:n])
						assert.NoError(t, rerr)
					}
				}
			}()
		}
	}()

	raddr, _ := listener.Addr().(*net.UDPAddr)

	// three clients writing three packets each,
	// server is batching 9 packets at a time, echoing back packets from client,
	// do two batches of writes from each client, two packets first cycle and one packet in second cycle
	//    - should not be able to read any packets after first write as the server is batching packets
	//    - should be able to read three packets from each client as server would have flushed nine packets
	// to ensure that write batch interval does not kick in, setting the write batch interval to a large value (1m)
	cc := 3
	var clients [3]*net.UDPConn
	wgs := sync.WaitGroup{}

	// first cycle, write two packets from each client,
	// should not be able to read any packets
	wgs.Add(cc)
	for i := range cc {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			defer wgs.Done()

			client, err := net.DialUDP("udp", nil, raddr)
			assert.NoError(t, err)
			clients[idx] = client

			for range 2 {
				_, err = client.Write([]byte(sendStr))
				assert.NoError(t, err)
			}

			err = client.SetReadDeadline(time.Now().Add(time.Second))
			assert.NoError(t, err)

			buf := make([]byte, 1400)
			n, err := client.Read(buf)
			assert.Zero(t, n, "unexpected packet from client: %d", idx)
			assert.ErrorContains(t, err, "timeout", "expected timeout from client: %d", idx)
		}()
	}
	wgs.Wait()

	// second cycle, write two packets from each client,
	// should be able to read three packets on average per client
	// (ordering is not guaranteed due to goroutine scheduling, so just check for a total of 9 packets)
	wgs.Add(cc * 3)
	for i := range cc {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			for range 2 {
				_, err := clients[idx].Write([]byte(sendStr))
				assert.NoError(t, err)
			}

			buf := make([]byte, 1400)
			for j := 0; ; j++ {
				err := clients[idx].SetReadDeadline(time.Now().Add(time.Second))
				assert.NoError(t, err)

				n, err := clients[idx].Read(buf)
				if err == nil {
					assert.Equal(t, sendStr, string(buf[:n]), "mismatch in first read, client: %d, packet: %d", idx, j)
					wgs.Done()
				} else {
					break
				}
			}
		}()
	}
	wgs.Wait()

	// should not be able to read any packets as the next batch is not ready yet
	wgs.Add(cc)
	for i := range cc {
		idx := i
		go func() {
			defer wgs.Done()

			err := clients[idx].SetReadDeadline(time.Now().Add(time.Second))
			assert.NoError(t, err)

			buf := make([]byte, 1400)
			n, err := clients[idx].Read(buf)
			assert.Zero(t, n, "unexpected packet from client: %d", idx)
			assert.ErrorContains(t, err, "timeout", "expected timeout from client: %d", idx)
		}()
	}
	wgs.Wait()

	// third cycle, write two packets from each client,
	// should be able to read three packets on average per client
	wgs.Add(cc * 3)
	for i := range cc {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			defer func() { _ = clients[idx].Close() }()

			for range 2 {
				_, err := clients[idx].Write([]byte(sendStr))
				assert.NoError(t, err)
			}

			buf := make([]byte, 1400)
			for j := 0; ; j++ {
				err := clients[idx].SetReadDeadline(time.Now().Add(time.Second))
				assert.NoError(t, err)

				n, err := clients[idx].Read(buf)
				if err == nil {
					assert.Equal(t, sendStr, string(buf[:n]), "mismatch in second read, client: %d, packet: %d", idx, j)
					wgs.Done()
				} else {
					break
				}
			}
		}()
	}
	wgs.Wait()

	_ = listener.Close()
	serverConnWg.Wait()
}

// gatedBatchConn holds the first WriteBatch until released, so a test can fill the
// other batch while the flusher is busy.
type gatedBatchConn struct {
	entered chan struct{}
	release chan struct{}
	writes  chan int
	first   sync.Once
}

func (c *gatedBatchConn) WriteBatch(ms []ipv4.Message, _ int) (int, error) {
	c.first.Do(func() {
		close(c.entered)
		<-c.release
	})
	c.writes <- len(ms)

	return len(ms), nil
}

func (c *gatedBatchConn) ReadBatch([]ipv4.Message, int) (int, error) { return 0, nil }

func (c *gatedBatchConn) Close() error { return nil }

func TestPingPong_FlushesBatchFilledDuringFlush(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	const size = 4
	conn := &gatedBatchConn{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		writes:  make(chan int, 2),
	}
	// with a long interval, only the batch-full signal can flush the second batch in time
	pp := newPingPong(size, time.Minute, conn)
	raddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000}

	for range size {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte{1}, raddr))
	}
	<-conn.entered

	// fill the other batch while the flusher is inside WriteBatch
	for range size {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte{2}, raddr))
	}
	close(conn.release)

	assert.Equal(t, size, <-conn.writes)
	select {
	case n := <-conn.writes:
		assert.Equal(t, size, n)
	case <-time.After(time.Second):
		assert.Fail(t, "batch filled during a flush waited for the flush interval")
	}

	pp.Close()
}

// stepBatchConn holds every WriteBatch until the test releases it.
type stepBatchConn struct {
	entered chan int
	release chan struct{}
}

func (c *stepBatchConn) WriteBatch(ms []ipv4.Message, _ int) (int, error) {
	c.entered <- len(ms)
	<-c.release

	return len(ms), nil
}

func (c *stepBatchConn) ReadBatch([]ipv4.Message, int) (int, error) { return 0, nil }

func (c *stepBatchConn) Close() error { return nil }

func TestPingPong_SwapWakesAllWaitingWriters(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	const size = 4
	conn := &stepBatchConn{entered: make(chan int), release: make(chan struct{})}
	pp := newPingPong(size, time.Minute, conn)
	raddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000}

	// first batch goes to the socket and stays there, second batch fills up
	for range size {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte{1}, raddr))
	}
	assert.Equal(t, size, <-conn.entered)
	for range size {
		assert.Equal(t, 1, pp.EnqueueMessage([]byte{2}, raddr))
	}

	// both batches are busy, so these writers wait
	var done sync.WaitGroup
	for range size {
		done.Add(1)
		go func() {
			defer done.Done()
			assert.Equal(t, 1, pp.EnqueueMessage([]byte{3}, raddr))
		}()
	}

	// finishing the first flush swaps in the empty batch, then the flusher holds
	// in the second flush, so the writers can only get in through that one swap
	conn.release <- struct{}{}
	assert.Equal(t, size, <-conn.entered)

	writersDone := make(chan struct{})
	go func() {
		done.Wait()
		close(writersDone)
	}()
	select {
	case <-writersDone:
	case <-time.After(time.Second):
		assert.Fail(t, "writers were not woken by the batch swap")
	}

	close(conn.release)
	go func() {
		for range conn.entered {
		}
	}()
	pp.Close()
	close(conn.entered)
}

func TestPingPong_CloseReleasesWaitingWriters(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	conn := &stepBatchConn{entered: make(chan int), release: make(chan struct{})}
	pp := newPingPong(1, time.Minute, conn)
	raddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000}

	assert.Equal(t, 1, pp.EnqueueMessage([]byte{1}, raddr))
	assert.Equal(t, 1, <-conn.entered)
	assert.Equal(t, 1, pp.EnqueueMessage([]byte{2}, raddr))

	written := make(chan int)
	go func() {
		written <- pp.EnqueueMessage([]byte{3}, raddr)
	}()

	closed := make(chan struct{})
	go func() {
		pp.Close()
		close(closed)
	}()
	// let the flush finish only once Close has marked the pingPong closed
	for atomic.LoadInt32(&pp.closed) == 0 {
		time.Sleep(time.Millisecond)
	}
	close(conn.release)

	select {
	case n := <-written:
		assert.Equal(t, 0, n)
	case <-time.After(time.Second):
		assert.Fail(t, "writer waiting for a batch was not released by Close")
	}
	<-closed
}
