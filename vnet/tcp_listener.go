// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"math"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v4"
)

// listenerObserver is the interface that a TCPListener's host network must satisfy.
// It extends connObserver (so accepted connections can re-use the same observer) and
// adds the three map operations the listener needs during the handshake and close.
type listenerObserver interface {
	connObserver
	insertTCPConn(conn *TCPConn) error
	deleteTCPConn(conn *TCPConn) error
	deleteTCPListener(addr net.Addr) error
	getLog() logging.LeveledLogger
}

// TCPListener implements transport.TCPListener.
type TCPListener struct {
	locAddr *net.TCPAddr
	obs     listenerObserver

	acceptCh chan *TCPConn
	mu       sync.Mutex
	closed   bool
	timer    *time.Timer
}

var _ transport.TCPListener = &TCPListener{}

func newTCPListener(locAddr *net.TCPAddr, obs listenerObserver) (*TCPListener, error) {
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
	// NOTE: the lock is released before the SYN-ACK is sent. A concurrent
	// Close() can mark l.closed=true in this window, causing the SYN-ACK to be
	// sent to a closing listener. This is harmless: the onEstablished callback
	// below re-checks l.closed under the mutex and discards the connection if
	// the listener has since closed, so no resource leak occurs.

	dst := tcp.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
	src := tcp.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert

	// If listener is on 0.0.0.0, bind accepted conn to the destination IP.
	loc := &net.TCPAddr{IP: dst.IP, Port: l.locAddr.Port}
	rem := &net.TCPAddr{IP: src.IP, Port: src.Port}

	conn, err := newTCPConn(loc, rem, l.obs, l.obs.getLog(), func(c *TCPConn) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			_ = c.Close()

			return
		}
		select {
		case l.acceptCh <- c:
		default:
			_ = c.Close() // accept queue full, reject the connection
		}
	})
	if err != nil {
		return
	}

	conn.mu.Lock()
	conn.state = tcpStateSynReceived
	conn.mu.Unlock()

	// Register early so ACK/data finds the conn.
	if err := l.obs.insertTCPConn(conn); err != nil {
		// Duplicate SYN (e.g. from vnet's duplication filter) — ignore.
		return
	}

	// Send SYN-ACK
	synAck := newChunkTCP(loc, rem, tcpSYN|tcpACK)
	if err := l.obs.write(synAck); err != nil {
		// Failed to send SYN-ACK — remove the conn so it doesn't leak in tcpConns.
		_ = l.obs.deleteTCPConn(conn)

		return
	}
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
		case c, ok := <-l.acceptCh:
			if !ok {
				return nil, errUseClosedNetworkConn
			}

			return c, nil
		case <-l.timer.C:
			return nil, &net.OpError{Op: "accept", Net: tcp, Addr: l.locAddr, Err: errIOTimeout}
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

	_ = l.obs.deleteTCPListener(l.locAddr)
	close(l.acceptCh)

	// Drain any connections that were fully established and queued but never
	// accepted. Without this they would remain open in the simulated network's
	// tcpConns map and interfere with subsequent test cases that reuse the Net
	// or Router.
	// Guard on obs != nil: newTCPConn always sets obs (and returns an error if
	// it is nil), so a nil obs means the entry is a test stub, not a real conn.
	for c := range l.acceptCh {
		if c.obs != nil {
			_ = c.Close()
		}
	}

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
