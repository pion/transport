// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
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

	"github.com/pion/transport/v3/test"
	"github.com/stretchr/testify/assert"
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
	for i := 0; i < cc; i++ {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			defer wgs.Done()

			client, err := net.DialUDP("udp", nil, raddr)
			assert.NoError(t, err)
			clients[idx] = client

			for j := 0; j < 2; j++ {
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
	for i := 0; i < cc; i++ {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			for j := 0; j < 2; j++ {
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
	for i := 0; i < cc; i++ {
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
	for i := 0; i < cc; i++ {
		sendStr := fmt.Sprintf("hello %d", i)

		idx := i
		go func() {
			defer func() { _ = clients[idx].Close() }()

			for j := 0; j < 2; j++ {
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
