// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"math"
	"net"
	"sync"
	"time"

	"github.com/pion/transport/v4"
)

// TCPListener implements transport.TCPListener.
type TCPListener struct {
	locAddr *net.TCPAddr
	obs     *Net

	acceptCh chan *TCPConn
	mu       sync.Mutex
	closed   bool
	timer    *time.Timer
}

var _ transport.TCPListener = &TCPListener{}

func newTCPListener(locAddr *net.TCPAddr, obs *Net) (*TCPListener, error) {
	if obs == nil {
		return nil, errObsCannotBeNil
	}

	return &TCPListener{
		locAddr:  locAddr,
		obs:      obs,
		acceptCh: make(chan *TCPConn, 64),
		timer:    time.NewTimer(time.Duration(math.MaxInt64)),
	}, nil
}

func (l *TCPListener) onInboundSYN(tcp *chunkTCP) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()

		return
	}
	l.mu.Unlock()

	dst := tcp.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
	src := tcp.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert

	// If listener is on 0.0.0.0, bind accepted conn to the destination IP.
	loc := &net.TCPAddr{IP: dst.IP, Port: l.locAddr.Port}
	rem := &net.TCPAddr{IP: src.IP, Port: src.Port}

	conn, err := newTCPConn(loc, rem, l.obs, func(c *TCPConn) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			_ = c.Close()

			return
		}
		l.acceptCh <- c
	})
	if err != nil {
		return
	}

	conn.mu.Lock()
	conn.state = tcpStateSynReceived
	conn.mu.Unlock()

	// Register early so ACK/data finds the conn.
	_ = l.obs.tcpConns.insert(conn)

	// Send SYN-ACK
	synAck := newChunkTCP(loc, rem, tcpSYN|tcpACK)
	_ = l.obs.write(synAck)
}

// Accept waits for and returns the next connection to the listener.
func (l *TCPListener) Accept() (net.Conn, error) {
	return l.AcceptTCP()
}

// AcceptTCP waits for and returns the next TCP connection to the listener.
func (l *TCPListener) AcceptTCP() (transport.TCPConn, error) {
	for {
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()

			return nil, errUseClosedNetworkConn
		}
		l.mu.Unlock()

		select {
		case c := <-l.acceptCh:
			return c, nil
		case <-l.timer.C:
			return nil, &net.OpError{Op: "accept", Net: tcp, Addr: l.locAddr, Err: newTimeoutError("i/o timeout")}
		}
	}
}

// Close closes the listener. Any blocked Accept operations will be unblocked and return errors.
func (l *TCPListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()

		return errAlreadyClosed
	}
	l.closed = true
	l.mu.Unlock()

	_ = l.obs.tcpListeners.delete(l.locAddr)
	close(l.acceptCh)

	return nil
}

// Addr returns the listener's network address.
func (l *TCPListener) Addr() net.Addr {
	return l.locAddr
}

// SetDeadline sets the deadline for future Accept calls.
func (l *TCPListener) SetDeadline(t time.Time) error {
	var d time.Duration
	if t.IsZero() {
		d = time.Duration(math.MaxInt64)
	} else {
		d = time.Until(t)
	}
	l.timer.Reset(d)

	return nil
}
