// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package udp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/transport/v4/test"
	"github.com/stretchr/testify/assert"
)

var errHandshakeFailed = errors.New("handshake failed")

// Note: doesn't work since closing isn't propagated to the other side
// func TestNetTest(t *testing.T) {
//	lim := test.TimeOut(time.Minute*1 + time.Second*10)
//	defer lim.Stop()
//
//	nettest.TestConn(t, func() (c1, c2 net.Conn, stop func(), err error) {
//		listener, c1, c2, err = pipe()
//		if err != nil {
//			return nil, nil, nil, err
//		}
//		stop = func() {
//			c1.Close()
//			c2.Close()
//			listener.Close(1 * time.Second)
//		}
//		return
//	})
//}

func TestStressDuplex(t *testing.T) {
	// Limit runtime in case of deadlocks
	lim := test.TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := test.CheckRoutines(t)
	defer report()

	// Run the test
	stressDuplex(t)
}

func stressDuplex(t *testing.T) {
	t.Helper()

	listener, ca, cb, err := pipe()
	assert.NoError(t, err)

	defer func() {
		err = ca.Close()
		assert.NoError(t, err)
		err = cb.Close()
		assert.NoError(t, err)
		err = listener.Close()
		assert.NoError(t, err)
	}()

	opt := test.Options{
		MsgSize:  2048,
		MsgCount: 1, // Can't rely on UDP message order in CI
	}

	err = test.StressDuplex(ca, cb, opt)
	assert.NoError(t, err)
}

func TestListenerCloseTimeout(t *testing.T) {
	// Limit runtime in case of deadlocks
	lim := test.TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := test.CheckRoutines(t)
	defer report()

	listener, ca, _, err := pipe()
	assert.NoError(t, err)

	err = listener.Close()
	assert.NoError(t, err)

	// Close client after server closes to cleanup
	err = ca.Close()
	assert.NoError(t, err)
}

func TestListenerCloseUnaccepted(t *testing.T) {
	// Limit runtime in case of deadlocks
	lim := test.TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := test.CheckRoutines(t)
	defer report()

	const backlog = 2

	network, addr := getConfig()
	listener, err := (&ListenConfig{
		Backlog: backlog,
	}).Listen(network, addr)
	assert.NoError(t, err)

	for i := 0; i < backlog; i++ {
		addr, ok := listener.Addr().(*net.UDPAddr)
		assert.True(t, ok)
		conn, err := net.DialUDP(network, nil, addr)
		assert.NoError(t, err)
		_, err = conn.Write([]byte{byte(i)})
		assert.NoError(t, err)
		assert.NoError(t, conn.Close())
	}

	time.Sleep(100 * time.Millisecond) // Wait all packets being processed by readLoop

	// Unaccepted connections must be closed by listener.Close()
	assert.NoError(t, listener.Close())
}

func TestListenerAcceptFilter(t *testing.T) { //nolint:cyclop
	// Limit runtime in case of deadlocks
	lim := test.TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := test.CheckRoutines(t)
	defer report()

	testCases := map[string]struct {
		packet []byte
		accept bool
	}{
		"CreateConn": {
			packet: []byte{0xAA},
			accept: true,
		},
		"Discarded": {
			packet: []byte{0x00},
			accept: false,
		},
	}

	for name, testCase := range testCases {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			network, addr := getConfig()
			listener, err := (&ListenConfig{
				AcceptFilter: func(pkt []byte) bool {
					return pkt[0] == 0xAA
				},
			}).Listen(network, addr)
			assert.NoError(t, err)

			var wgAcceptLoop sync.WaitGroup
			wgAcceptLoop.Add(1)
			defer func() {
				assert.NoError(t, listener.Close())
				wgAcceptLoop.Wait()
			}()

			addr, ok := listener.Addr().(*net.UDPAddr)
			assert.True(t, ok)
			conn, err := net.DialUDP(network, nil, addr)
			assert.NoError(t, err)
			_, err = conn.Write(testCase.packet)
			assert.NoError(t, err)
			defer func() {
				assert.NoError(t, conn.Close())
			}()

			chAccepted := make(chan struct{})
			go func() {
				defer wgAcceptLoop.Done()

				conn, aArr := listener.Accept()
				if aArr != nil {
					assert.ErrorIs(t, aArr, ErrClosedListener)

					return
				}
				close(chAccepted)
				assert.NoError(t, conn.Close())
			}()

			var accepted bool
			select {
			case <-chAccepted:
				accepted = true
			case <-time.After(10 * time.Millisecond):
			}

			if testCase.accept {
				assert.Equal(t, testCase.accept, accepted, "Packet should create new conn")
			} else {
				assert.Equal(t, testCase.accept, accepted, "Packet should not create new conn")
			}
		})
	}
}

func TestListenerConcurrent(t *testing.T) { //nolint:cyclop
	// Limit runtime in case of deadlocks
	lim := test.TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := test.CheckRoutines(t)
	defer report()

	const backlog = 2

	network, addr := getConfig()
	listener, err := (&ListenConfig{
		Backlog: backlog,
	}).Listen(network, addr)
	assert.NoError(t, err)

	for i := 0; i < backlog+1; i++ {
		addr, ok := listener.Addr().(*net.UDPAddr)
		assert.True(t, ok)
		conn, connErr := net.DialUDP(network, nil, addr)
		assert.NoError(t, connErr)
		_, connErr = conn.Write([]byte{byte(i)})
		assert.NoError(t, connErr)
		assert.NoError(t, conn.Close())
	}

	time.Sleep(100 * time.Millisecond) // Wait all packets being processed by readLoop

	for i := 0; i < backlog; i++ {
		conn, connErr := listener.Accept()
		assert.NoError(t, connErr)
		b := make([]byte, 1)
		n, connErr := conn.Read(b)
		assert.NoError(t, connErr)
		assert.Equalf(t, []byte{byte(i)}, b[:n], "Packet from connection %d is wrong", i)
		assert.NoError(t, conn.Close())
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, connErr := listener.Accept()
		assert.ErrorIs(t, connErr, ErrClosedListener, "Connection exceeding backlog limit must be discarded")

		if connErr == nil {
			_ = conn.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond) // Last Accept should be discarded
	err = listener.Close()
	assert.NoError(t, err)

	wg.Wait()
}

func pipe() (net.Listener, net.Conn, *net.UDPConn, error) {
	// Start listening
	network, addr := getConfig()
	listener, err := Listen(network, addr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Open a connection
	var dConn *net.UDPConn
	addr, ok := listener.Addr().(*net.UDPAddr)
	if !ok {
		return nil, nil, nil, fmt.Errorf("failed to get listener addr: %w", os.ErrInvalid)
	}

	dConn, err = net.DialUDP(network, nil, addr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to dial: %w", err)
	}

	// Write to the connection to initiate it
	handshake := "hello"
	_, err = dConn.Write([]byte(handshake))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to write to dialed Conn: %w", err)
	}

	// Accept the connection
	var lConn net.Conn
	lConn, err = listener.Accept()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to accept Conn: %w", err)
	}

	var n int
	buf := make([]byte, len(handshake))
	if n, err = lConn.Read(buf); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read handshake: %w", err)
	}

	result := string(buf[:n])
	if handshake != result {
		return nil, nil, nil, fmt.Errorf("%w: %s != %s", errHandshakeFailed, handshake, result)
	}

	return listener, lConn, dConn, nil
}

func getConfig() (string, *net.UDPAddr) {
	return "udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func TestConnClose(t *testing.T) { //nolint:cyclop
	lim := test.TimeOut(time.Second * 5)
	defer lim.Stop()

	t.Run("Close", func(t *testing.T) {
		// Check for leaking routines
		report := test.CheckRoutines(t)
		defer report()

		l, ca, cb, err := pipe()
		assert.NoError(t, err)
		assert.NoError(t, ca.Close(), "Failed to close A side")
		assert.NoError(t, cb.Close(), "Failed to close B side")
		assert.NoError(t, l.Close(), "Failed to close listener")
	})
	t.Run("CloseError1", func(t *testing.T) {
		// Check for leaking routines
		report := test.CheckRoutines(t)
		defer report()

		listn, ca, cb, err := pipe()
		assert.NoError(t, err)
		// Close l.pConn to inject error.
		list, ok := listn.(*listener)
		assert.True(t, ok)
		assert.NoError(t, list.pConn.Close())

		assert.NoError(t, ca.Close(), "Failed to close A side")
		assert.NoError(t, cb.Close(), "Failed to close B side")
		assert.Error(t, listn.Close(), "Error is not propagated to Listener.Close")
	})
	t.Run("CloseError2", func(t *testing.T) {
		// Check for leaking routines
		report := test.CheckRoutines(t)
		defer report()

		l, ca, cb, err := pipe()
		assert.NoError(t, err)
		// Close l.pConn to inject error.
		list, ok := l.(*listener)
		assert.True(t, ok)
		assert.NoError(t, list.pConn.Close())

		assert.NoError(t, cb.Close(), "Failed to close B side")
		assert.NoError(t, l.Close(), "Failed to close listener")
		assert.Error(t, ca.Close(), "Error is not propagated to Conn.Close")
	})
	t.Run("CancelRead", func(t *testing.T) {
		// Limit runtime in case of deadlocks
		lim := test.TimeOut(time.Second * 5)
		defer lim.Stop()

		// Check for leaking routines
		report := test.CheckRoutines(t)
		defer report()

		listn, ca, cb, err := pipe()
		assert.NoError(t, err)

		errC := make(chan error, 1)
		go func() {
			buf := make([]byte, 1024)
			// This read will block because we don't write on the other side.
			// Calling Close must unblock the call.
			_, err := ca.Read(buf)
			errC <- err
		}()

		assert.NoError(t, ca.Close(), "Failed to close A side")

		// Main test condition, Read should return
		// after ca.Close() by closing the buffer.
		assert.ErrorIs(t, <-errC, io.EOF)
		assert.NoError(t, cb.Close(), "Failed to close A side")
		assert.NoError(t, listn.Close(), "Failed to close listener")
	})
}

func TestBatchIO(t *testing.T) {
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

	wgs := sync.WaitGroup{}
	cc := 3
	wgs.Add(cc)

	for i := 0; i < cc; i++ {
		sendStr := fmt.Sprintf("hello %d", i)
		go func() {
			defer wgs.Done()
			buf := make([]byte, 1400)
			client, err := net.DialUDP("udp", nil, raddr)
			assert.NoError(t, err)
			defer func() { _ = client.Close() }()
			for i := 0; i < 1; i++ {
				_, err := client.Write([]byte(sendStr))
				assert.NoError(t, err)
				err = client.SetReadDeadline(time.Now().Add(time.Second))
				assert.NoError(t, err)
				n, err := client.Read(buf)
				assert.NoError(t, err)
				assert.Equal(t, sendStr, string(buf[:n]), "mismatch in client: %d", i)
			}
		}()
	}
	wgs.Wait()

	_ = listener.Close()
	serverConnWg.Wait()
}
