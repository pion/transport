// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pion/transport/v4"
)

type tcpState uint8

const (
	tcpStateInit tcpState = iota
	tcpStateSynSent
	tcpStateSynReceived
	tcpStateEstablished
	tcpStateClosed
)

var (
	errConnectionReset          = errors.New("connection reset")
	errConnectionNotEstablished = errors.New("connection not established")
)

// TCPConn implements transport.TCPConn.
type TCPConn struct {
	locAddr *net.TCPAddr
	remAddr *net.TCPAddr
	obs     connObserver

	mu            sync.Mutex
	state         tcpState
	inboundCh     chan tcpSegment
	curSeg        *tcpSegment
	curSegOffset  int
	readClosed    bool // remote has closed
	readChClosed  bool
	writeClosed   bool
	closed        bool
	readDeadline  time.Time
	writeDeadline time.Time

	nextSeq     uint32
	pendingAcks map[uint32]chan struct{}

	// client connect flow
	establishedCh chan struct{}
	establishErr  error

	// server side: notify listener once established
	onEstablished func(*TCPConn)
}

type tcpSegment struct {
	seq  uint32
	data []byte
}

const tcpInboundQueueSize = 10

var _ transport.TCPConn = &TCPConn{}

func newTCPConn(locAddr, remAddr *net.TCPAddr, obs connObserver, onEstablished func(*TCPConn)) (*TCPConn, error) {
	if obs == nil {
		return nil, errObsCannotBeNil
	}

	conn := &TCPConn{
		locAddr:       locAddr,
		remAddr:       remAddr,
		obs:           obs,
		state:         tcpStateInit,
		inboundCh:     make(chan tcpSegment, tcpInboundQueueSize),
		establishedCh: make(chan struct{}),
		onEstablished: onEstablished,
		readClosed:    false,
		writeClosed:   false,
		closed:        false,
		nextSeq:       0,
		pendingAcks:   map[uint32]chan struct{}{},
	}

	return conn, nil
}

func (c *TCPConn) startClientHandshake() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return errUseClosedNetworkConn
	}
	if c.locAddr.IP == nil {
		c.mu.Unlock()

		return errLocAddr
	}

	c.state = tcpStateSynSent
	c.mu.Unlock()

	src := &net.TCPAddr{IP: c.locAddr.IP, Port: c.locAddr.Port}
	dst := &net.TCPAddr{IP: c.remAddr.IP, Port: c.remAddr.Port}

	syn := newChunkTCP(src, dst, tcpSYN)

	return c.obs.write(syn)
}

func (c *TCPConn) waitEstablished() error {
	<-c.establishedCh
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.establishErr
}

func (c *TCPConn) onInboundChunk(chunk *chunkTCP) { // nolint:cyclop,gocognit
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	// RST aborts connection immediately
	if chunk.flags&tcpRST != 0 {
		c.establishErr = &net.OpError{Op: "dial", Net: tcp, Addr: c.remAddr, Err: errConnectionReset}
		c.closed = true
		c.state = tcpStateClosed
		c.readClosed = true
		c.closeReadChLocked()
		c.closePendingAcksLocked()
		select {
		case <-c.establishedCh:
		default:
			close(c.establishedCh)
		}

		return
	}

	// handshake
	if c.state == tcpStateSynSent {
		if chunk.flags&(tcpSYN|tcpACK) == (tcpSYN | tcpACK) {
			c.state = tcpStateEstablished
			// reply ACK
			src := &net.TCPAddr{IP: c.locAddr.IP, Port: c.locAddr.Port}
			dst := &net.TCPAddr{IP: c.remAddr.IP, Port: c.remAddr.Port}
			ack := newChunkTCP(src, dst, tcpACK)
			go func() { _ = c.obs.write(ack) }()
			select {
			case <-c.establishedCh:
			default:
				close(c.establishedCh)
			}

			return
		}
	}

	if c.state == tcpStateSynReceived {
		if chunk.flags&tcpACK != 0 && chunk.flags&tcpSYN == 0 {
			c.state = tcpStateEstablished
			cb := c.onEstablished
			if cb != nil {
				go cb(c)
			}
			// Do not return here; the first ACK may also carry data (PSH).
		}
	}

	if chunk.flags&tcpFIN != 0 {
		c.readClosed = true
		select {
		case <-c.establishedCh:
		default:
			// if the other side closed before connect completed
			c.establishErr = io.EOF
			close(c.establishedCh)
		}
		c.closeReadChLocked()

		return
	}

	// Data ACK
	if chunk.flags&tcpACK != 0 && chunk.ackNum != 0 {
		if ch, ok := c.pendingAcks[chunk.ackNum]; ok {
			delete(c.pendingAcks, chunk.ackNum)
			close(ch)
		}
		// ACK may accompany other flags (e.g. PSH), so don't return.
	}

	if chunk.flags&tcpPSH != 0 && len(chunk.userData) > 0 {
		payload := make([]byte, len(chunk.userData))
		copy(payload, chunk.userData)
		if !c.readChClosed {
			seg := tcpSegment{seq: chunk.seqNum, data: payload}
			select {
			case c.inboundCh <- seg:
			default:
				// drop if the receive queue is full
			}
		}

		return
	}
}

func (c *TCPConn) closeReadChLocked() {
	if c.readChClosed {
		return
	}
	c.readChClosed = true
	close(c.inboundCh)
}

func (c *TCPConn) closePendingAcksLocked() {
	for _, ch := range c.pendingAcks {
		close(ch)
	}
	clear(c.pendingAcks)
}

// Read reads data from the connection.
func (c *TCPConn) Read(b []byte) (int, error) { // nolint:gocognit,cyclop
	for {
		var ack *chunkTCP
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()

			return 0, &net.OpError{Op: "read", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
		}
		// Serve current segment if present.
		if c.curSeg != nil {
			remaining := c.curSeg.data[c.curSegOffset:]
			n := copy(b, remaining)
			c.curSegOffset += n
			if c.curSegOffset >= len(c.curSeg.data) {
				// ACK after the segment has been read.
				src := &net.TCPAddr{IP: c.locAddr.IP, Port: c.locAddr.Port}
				dst := &net.TCPAddr{IP: c.remAddr.IP, Port: c.remAddr.Port}
				ack = newChunkTCP(src, dst, tcpACK)
				ack.ackNum = c.curSeg.seq
				c.curSeg = nil
				c.curSegOffset = 0
			}
			c.mu.Unlock()
			if ack != nil {
				_ = c.obs.write(ack)
			}

			return n, nil
		}

		inboundCh := c.inboundCh
		deadline := c.readDeadline
		c.mu.Unlock()

		// Wait for the next segment.
		if !deadline.IsZero() { // nolint:nestif
			until := time.Until(deadline)
			if until <= 0 {
				return 0, &net.OpError{Op: "read", Net: tcp, Addr: c.locAddr, Err: newTimeoutError("i/o timeout")}
			}
			timer := time.NewTimer(until)
			select {
			case seg, ok := <-inboundCh:
				if !timer.Stop() {
					<-timer.C
				}
				if !ok {
					c.mu.Lock()
					defer c.mu.Unlock()
					if c.closed {
						return 0, &net.OpError{Op: "read", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
					}

					return 0, io.EOF
				}
				c.mu.Lock()
				c.curSeg = &seg
				c.curSegOffset = 0
				c.mu.Unlock()

				continue
			case <-timer.C:
				return 0, &net.OpError{Op: "read", Net: tcp, Addr: c.locAddr, Err: newTimeoutError("i/o timeout")}
			}
		}

		seg, ok := <-inboundCh
		if !ok {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed {
				return 0, &net.OpError{Op: "read", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
			}

			return 0, io.EOF
		}
		c.mu.Lock()
		c.curSeg = &seg
		c.curSegOffset = 0
		c.mu.Unlock()
	}
}

// Write writes data to the connection.
func (c *TCPConn) Write(b []byte) (int, error) { // nolint:cyclop
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return 0, &net.OpError{Op: "write", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
	}
	if c.writeClosed {
		c.mu.Unlock()

		return 0, io.ErrClosedPipe
	}
	if c.state != tcpStateEstablished {
		c.mu.Unlock()

		return 0, errConnectionNotEstablished
	}

	seq := c.nextSeq + 1
	c.nextSeq = seq
	ackCh := make(chan struct{})
	c.pendingAcks[seq] = ackCh
	deadline := c.writeDeadline

	payload := make([]byte, len(b))
	copy(payload, b)
	src := &net.TCPAddr{IP: c.locAddr.IP, Port: c.locAddr.Port}
	dst := &net.TCPAddr{IP: c.remAddr.IP, Port: c.remAddr.Port}
	chunk := newChunkTCP(src, dst, tcpPSH|tcpACK)
	chunk.userData = payload
	chunk.seqNum = seq
	c.mu.Unlock()

	if err := c.obs.write(chunk); err != nil {
		c.mu.Lock()
		if ch, ok := c.pendingAcks[seq]; ok {
			delete(c.pendingAcks, seq)
			close(ch)
		}
		c.mu.Unlock()

		return 0, err
	}

	if !deadline.IsZero() {
		until := time.Until(deadline)
		if until <= 0 {
			return 0, &net.OpError{Op: "write", Net: tcp, Addr: c.locAddr, Err: newTimeoutError("i/o timeout")}
		}
		timer := time.NewTimer(until)
		select {
		case <-ackCh:
			if !timer.Stop() {
				<-timer.C
			}
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return 0, &net.OpError{Op: "write", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
			}

			return len(b), nil
		case <-timer.C:
			c.mu.Lock()
			if ch, ok := c.pendingAcks[seq]; ok {
				delete(c.pendingAcks, seq)
				close(ch)
			}
			c.mu.Unlock()

			return 0, &net.OpError{Op: "write", Net: tcp, Addr: c.locAddr, Err: newTimeoutError("i/o timeout")}
		}
	}

	<-ackCh
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return 0, &net.OpError{Op: "write", Net: tcp, Addr: c.locAddr, Err: errUseClosedNetworkConn}
	}

	return len(b), nil
}

// Close closes the connection.
func (c *TCPConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return errAlreadyClosed
	}
	c.closed = true
	c.state = tcpStateClosed
	c.readClosed = true
	c.writeClosed = true
	c.closeReadChLocked()
	c.closePendingAcksLocked()
	c.mu.Unlock()

	// Best-effort FIN
	_ = c.CloseWrite()
	if n, ok := c.obs.(*Net); ok {
		_ = n.tcpConns.deleteConn(c)
	}

	return nil
}

// CloseRead closes the read side of the connection.
func (c *TCPConn) CloseRead() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readClosed = true
	c.closeReadChLocked()

	return nil
}

// CloseWrite closes the write side of the connection.
func (c *TCPConn) CloseWrite() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return errUseClosedNetworkConn
	}
	if c.writeClosed {
		c.mu.Unlock()

		return nil
	}
	c.writeClosed = true
	src := &net.TCPAddr{IP: c.locAddr.IP, Port: c.locAddr.Port}
	dst := &net.TCPAddr{IP: c.remAddr.IP, Port: c.remAddr.Port}
	fin := newChunkTCP(src, dst, tcpFIN|tcpACK)
	c.mu.Unlock()

	return c.obs.write(fin)
}

// LocalAddr returns the local network address.
func (c *TCPConn) LocalAddr() net.Addr {
	return c.locAddr
}

// RemoteAddr returns the remote network address.
func (c *TCPConn) RemoteAddr() net.Addr {
	return c.remAddr
}

// SetDeadline sets the read and write deadlines associated with the connection.
func (c *TCPConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}

	return c.SetWriteDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls.
func (c *TCPConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.mu.Unlock()

	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
func (c *TCPConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()

	return nil
}

// ReadFrom reads data from r and writes it to the connection.
func (c *TCPConn) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(c, r)
}

// SetLinger sets the behavior of Close method on a connection with pending data.
func (c *TCPConn) SetLinger(int) error {
	return transport.ErrNotSupported
}

// SetKeepAlive enables or disables the keep-alive functionality for this connection.
func (c *TCPConn) SetKeepAlive(bool) error {
	return transport.ErrNotSupported
}

// SetKeepAlivePeriod sets the period between keep-alive messages for this connection.
func (c *TCPConn) SetKeepAlivePeriod(time.Duration) error {
	return transport.ErrNotSupported
}

// SetNoDelay enables or disables the Nagle's algorithm for this connection.
func (c *TCPConn) SetNoDelay(bool) error {
	return transport.ErrNotSupported
}

// SetWriteBuffer sets the size of the operating system's transmit buffer associated
// with the connection.
func (c *TCPConn) SetWriteBuffer(int) error {
	return transport.ErrNotSupported
}

// SetReadBuffer sets the size of the operating system's receive buffer associated
// with the connection.
func (c *TCPConn) SetReadBuffer(int) error {
	return transport.ErrNotSupported
}
