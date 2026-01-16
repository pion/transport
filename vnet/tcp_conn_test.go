// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFailedToConvertToChunkTCP = errors.New("failed to convert chunk to chunkTCP")

func newAckingEchoTCPObserver(connPtr **TCPConn) *dummyObserver {
	return &dummyObserver{
		onWrite: func(c Chunk) error {
			conn := *connPtr
			if conn == nil {
				return errors.New("tcp conn is nil") // nolint:err113
			}

			tc, ok := c.(*chunkTCP)
			if !ok {
				return errFailedToConvertToChunkTCP
			}

			// Immediately ACK the sent segment as if the remote read it.
			if tc.flags&tcpPSH != 0 && tc.seqNum != 0 {
				dstAddr := tc.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
				srcAddr := tc.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert
				ack := newChunkTCP(dstAddr, srcAddr, tcpACK)
				ack.ackNum = tc.seqNum
				conn.onInboundChunk(ack)
			}

			// Echo back payload as if it came from the remote.
			dstAddr := tc.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
			srcAddr := tc.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert
			echo := newChunkTCP(dstAddr, srcAddr, tcpPSH|tcpACK)
			echo.userData = make([]byte, len(tc.userData))
			copy(echo.userData, tc.userData)
			conn.onInboundChunk(echo)

			return nil
		},
		onOnClosed: func(net.Addr) {},
	}
}

func TestTCPConn(t *testing.T) { //nolint:cyclop,maintidx,gocyclo
	log := logging.NewDefaultLoggerFactory().NewLogger("test")

	t.Run("ReadFrom Read", func(t *testing.T) {
		var conn *TCPConn
		data := []byte("Hello")
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := newAckingEchoTCPObserver(&conn)

		var err error
		conn, err = newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		rcvdCh := make(chan struct{})
		doneCh := make(chan struct{})

		go func() {
			buf := make([]byte, 1500)

			for {
				n, err2 := conn.Read(buf)
				if err2 != nil {
					log.Debug("conn closed. exiting the read loop")

					break
				}
				log.Debug("read data")
				assert.Equal(t, len(data), n, "should match")
				assert.Equal(t, string(data), string(buf[:n]), "should match")
				rcvdCh <- struct{}{}
			}

			close(doneCh)
		}()

		n, err := conn.ReadFrom(bytes.NewReader(data))
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		assert.Equal(t, int64(len(data)), n, "should match")

	loop:
		for {
			select {
			case <-rcvdCh:
				log.Debug("closing conn..")
				err2 := conn.Close()
				assert.Nil(t, err2, "should succeed")
			case <-doneCh:
				break loop
			}
		}
	})

	t.Run("Write Read", func(t *testing.T) {
		var conn *TCPConn
		var err error
		data := []byte("Hello")
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := newAckingEchoTCPObserver(&conn)

		conn, err = newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		rcvdCh := make(chan struct{})
		doneCh := make(chan struct{})

		go func() {
			buf := make([]byte, 1500)

			for {
				n, err2 := conn.Read(buf)
				if err2 != nil {
					log.Debug("conn closed. exiting the read loop")

					break
				}
				log.Debug("read data")
				assert.Equal(t, len(data), n, "should match")
				assert.Equal(t, string(data), string(buf[:n]), "should match")
				rcvdCh <- struct{}{}
			}

			close(doneCh)
		}()

		var n int
		n, err = conn.Write(data)
		if !assert.Nil(t, err, "should succeed") {
			return
		}
		assert.Equal(t, len(data), n, "should match")

	loop:
		for {
			select {
			case <-rcvdCh:
				log.Debug("closing conn..")
				err = conn.Close()
				assert.Nil(t, err, "should succeed")
			case <-doneCh:
				break loop
			}
		}
	})

	deadlineTest := func(t *testing.T, readOnly bool) {
		t.Helper()

		var conn *TCPConn
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite:    func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {},
		}

		var err error
		conn, err = newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		doneCh := make(chan struct{})

		if readOnly {
			err = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		} else {
			err = conn.SetDeadline(time.Now().Add(50 * time.Millisecond))
		}
		assert.Nil(t, err, "should succeed")

		go func() {
			buf := make([]byte, 1500)
			_, err2 := conn.Read(buf)
			assert.NotNil(t, err2, "should return error")
			var ne *net.OpError
			if errors.As(err2, &ne) {
				assert.True(t, ne.Timeout(), "should be a timeout")
			} else {
				assert.True(t, false, "should be an net.OpError")
			}

			assert.Nil(t, conn.Close(), "should succeed")
			close(doneCh)
		}()

		<-doneCh
	}

	t.Run("SetReadDeadline", func(t *testing.T) {
		deadlineTest(t, true)
	})

	t.Run("SetDeadline", func(t *testing.T) {
		deadlineTest(t, false)
	})

	t.Run("SetWriteDeadline", func(t *testing.T) {
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite:    func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {},
		}

		conn, err := newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		err = conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		assert.NoError(t, err, "should succeed")

		_, err = conn.Write([]byte("blocked"))
		assert.Error(t, err, "should timeout")
		var ne *net.OpError
		if errors.As(err, &ne) {
			assert.True(t, ne.Timeout(), "should be a timeout")
		} else {
			assert.True(t, false, "should be a net.OpError")
		}

		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("Write blocks until peer reads", func(t *testing.T) {
		msg := []byte("Hello")
		addrA := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		addrB := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		var connA *TCPConn
		var connB *TCPConn

		obsA := &dummyObserver{
			onWrite: func(c Chunk) error {
				tc, ok := c.(*chunkTCP)
				if !ok {
					return errFailedToConvertToChunkTCP
				}
				// Deliver to peer.
				connB.onInboundChunk(tc.Clone().(*chunkTCP)) //nolint:forcetypeassert

				return nil
			},
			onOnClosed: func(net.Addr) {},
		}

		obsB := &dummyObserver{
			onWrite: func(c Chunk) error {
				tc, ok := c.(*chunkTCP)
				if !ok {
					return errFailedToConvertToChunkTCP
				}
				// Deliver to peer.
				connA.onInboundChunk(tc.Clone().(*chunkTCP)) //nolint:forcetypeassert

				return nil
			},
			onOnClosed: func(net.Addr) {},
		}

		var err error
		connA, err = newTCPConn(addrA, addrB, obsA, nil)
		assert.NoError(t, err, "should succeed")
		connB, err = newTCPConn(addrB, addrA, obsB, nil)
		assert.NoError(t, err, "should succeed")

		connA.mu.Lock()
		connA.state = tcpStateEstablished
		connA.mu.Unlock()
		connB.mu.Lock()
		connB.state = tcpStateEstablished
		connB.mu.Unlock()

		writeDone := make(chan error, 1)
		go func() {
			_, err2 := connA.Write(msg)
			writeDone <- err2
		}()

		// Should still be blocked (no read => no ACK).
		select {
		case err2 := <-writeDone:
			assert.Fail(t, "Write returned before peer read", "%v", err2)

			return
		case <-time.After(200 * time.Millisecond):
		}

		_ = connB.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, len(msg))
		_, err = io.ReadFull(connB, buf)
		assert.NoError(t, err, "should succeed")
		assert.Equal(t, msg, buf, "should match")

		select {
		case err2 := <-writeDone:
			assert.NoError(t, err2, "should succeed")
		case <-time.After(2 * time.Second):
			assert.Fail(t, "Write did not unblock after peer read")

			return
		}

		assert.NoError(t, connA.Close(), "should succeed")
		assert.NoError(t, connB.Close(), "should succeed")
	})

	t.Run("RST", func(t *testing.T) {
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite:    func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {},
		}

		conn, err := newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		writeDone := make(chan error, 1)
		go func() {
			_, err2 := conn.Write([]byte("data"))
			writeDone <- err2
		}()

		select {
		case err2 := <-writeDone:
			assert.Fail(t, "Write returned before RST", "%v", err2)

			return
		case <-time.After(100 * time.Millisecond):
		}

		rst := newChunkTCP(dstAddr, srcAddr, tcpRST)
		conn.onInboundChunk(rst)

		select {
		case err2 := <-writeDone:
			assert.Error(t, err2, "should error")
			var ne *net.OpError
			if errors.As(err2, &ne) {
				assert.Equal(t, "write", ne.Op, "should match")
				assert.Equal(t, errUseClosedNetworkConn, ne.Err, "should match")
			} else {
				assert.True(t, false, "should be a net.OpError")
			}
		case <-time.After(2 * time.Second):
			assert.Fail(t, "Write did not unblock after RST")

			return
		}

		buf := make([]byte, 10)
		_, err = conn.Read(buf)
		assert.Error(t, err, "should error")
		_, err = conn.Write([]byte("x"))
		assert.Error(t, err, "should error")
	})

	t.Run("ReadClosed (CloseRead)", func(t *testing.T) {
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		var conn *TCPConn
		obs := &dummyObserver{
			onWrite: func(c Chunk) error {
				tc, ok := c.(*chunkTCP)
				if !ok {
					return errFailedToConvertToChunkTCP
				}
				// ACK writes so Write doesn't block.
				if tc.flags&tcpPSH != 0 && tc.seqNum != 0 {
					dstAddr := tc.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
					srcAddr := tc.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert
					ack := newChunkTCP(dstAddr, srcAddr, tcpACK)
					ack.ackNum = tc.seqNum
					conn.onInboundChunk(ack)
				}

				return nil
			},
			onOnClosed: func(net.Addr) {},
		}

		var err error
		conn, err = newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")
		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		assert.NoError(t, conn.CloseRead(), "should succeed")

		buf := make([]byte, 10)
		_, err = conn.Read(buf)
		assert.Equal(t, io.EOF, err, "should EOF")

		// Write side still usable until closed.
		_, err = conn.Write([]byte("ok"))
		assert.NoError(t, err, "should succeed")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("ReadClosed (remote FIN)", func(t *testing.T) {
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite:    func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {},
		}

		conn, err := newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")
		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		fin := newChunkTCP(dstAddr, srcAddr, tcpFIN|tcpACK)
		conn.onInboundChunk(fin)

		buf := make([]byte, 10)
		_, err = conn.Read(buf)
		assert.Equal(t, io.EOF, err, "should EOF")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("WriteClosed (CloseWrite)", func(t *testing.T) {
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite:    func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {},
		}

		conn, err := newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")
		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		assert.NoError(t, conn.CloseWrite(), "should succeed")
		_, err = conn.Write([]byte("nope"))
		assert.Equal(t, io.ErrClosedPipe, err, "should match")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("Inbound during close", func(t *testing.T) {
		var nClosed int32
		var conn *TCPConn
		srcAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		dstAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}

		obs := &dummyObserver{
			onWrite: func(Chunk) error { return nil },
			onOnClosed: func(net.Addr) {
				atomic.AddInt32(&nClosed, 1)
			},
		}

		var err error
		conn, err = newTCPConn(srcAddr, dstAddr, obs, nil)
		assert.NoError(t, err, "should succeed")

		conn.mu.Lock()
		conn.state = tcpStateEstablished
		conn.mu.Unlock()

		fin := newChunkTCP(dstAddr, srcAddr, tcpFIN|tcpACK)
		psh := newChunkTCP(dstAddr, srcAddr, tcpPSH|tcpACK)
		psh.userData = []byte("x")

		for i := 0; i < 1000; i++ { // nolint:staticcheck // (false positive detection)
			chDone := make(chan struct{})
			go func() {
				time.Sleep(20 * time.Millisecond)
				assert.NoError(t, conn.Close())
				close(chDone)
			}()
			tick := time.NewTicker(10 * time.Millisecond)
			for {
				defer tick.Stop()
				select {
				case <-chDone:
					// TCPConn doesn't currently notify the observer via onClosed.
					assert.Equal(t, int32(0), atomic.LoadInt32(&nClosed), "should not invoke onClosed")

					return
				case <-tick.C:
					conn.onInboundChunk(psh)
					conn.onInboundChunk(fin)
				}
			}
		}
	})
}

func TestVNetTCPDialListen(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	router, err := NewRouter(&RouterConfig{
		CIDR:          "192.0.2.0/24",
		LoggerFactory: loggerFactory,
	})
	require.NoError(t, err)
	require.NoError(t, router.Start())
	defer func() { _ = router.Stop() }()

	serverNet, err := NewNet(&NetConfig{})
	require.NoError(t, err)
	clientNet, err := NewNet(&NetConfig{})
	require.NoError(t, err)

	require.NoError(t, router.AddNet(serverNet))
	require.NoError(t, router.AddNet(clientNet))

	// Bind listener to server's assigned eth0 address so clients can route to it.
	eth0, err := serverNet.InterfaceByName("eth0")
	require.NoError(t, err)
	addrs, err := eth0.Addrs()
	require.NoError(t, err)
	require.NotEmpty(t, addrs)
	serverIP := addrs[0].(*net.IPNet).IP //nolint:forcetypeassert

	ln, err := serverNet.ListenTCP(tcp4, &net.TCPAddr{IP: serverIP, Port: 0})
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	srvAddr := ln.Addr().(*net.TCPAddr) //nolint:forcetypeassert

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		c, err2 := ln.AcceptTCP()
		require.NoError(t, err2)
		defer func() { _ = c.Close() }()

		buf := make([]byte, 5)
		_, err2 = io.ReadFull(c, buf)
		require.NoError(t, err2)
		require.Equal(t, []byte("hello"), buf)

		_, err2 = c.Write([]byte("world"))
		require.NoError(t, err2)
	}()

	conn, err := clientNet.DialTCP(tcp4, nil, &net.TCPAddr{IP: srvAddr.IP, Port: srvAddr.Port})
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, []byte("world"), buf)

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "server did not finish")
	}
}
