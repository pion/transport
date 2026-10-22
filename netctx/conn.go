// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package netctx wraps common net interfaces using context.Context.
package netctx

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// ErrClosing is returned on Write to closed connection.
var ErrClosing = errors.New("use of closed network connection")

// Reader is an interface for context controlled reader.
type Reader interface {
	ReadContext(context.Context, []byte) (int, error)
}

// Writer is an interface for context controlled writer.
type Writer interface {
	WriteContext(context.Context, []byte) (int, error)
}

// ReadWriter is a composite of ReadWriter.
type ReadWriter interface {
	Reader
	Writer
}

// Conn is a wrapper of net.Conn using context.Context.
type Conn interface {
	Reader
	Writer
	io.Closer
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Conn() net.Conn
}

type conn struct {
	nextConn  net.Conn
	closed    chan struct{}
	closeOnce sync.Once
	readMu    sync.Mutex
	writeMu   sync.Mutex
}

var veryOld = time.Unix(0, 1) //nolint:gochecknoglobals

var finishNothing = func() error { return nil } //nolint:gochecknoglobals

// cancelOnDone makes a blocked call return by moving its deadline into the past
// once ctx is done. Call the returned function after the call to undo that.
func cancelOnDone(ctx context.Context, setDeadline func(time.Time) error) func() error {
	if ctx.Done() == nil {
		return finishNothing
	}
	var errSet error
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		errSet = setDeadline(veryOld)
		close(canceled)
	})

	return func() error {
		if stop() {
			return nil
		}
		<-canceled
		if errSet != nil {
			return errSet
		}

		return setDeadline(time.Time{})
	}
}

// NewConn creates a new Conn wrapping given net.Conn.
func NewConn(netConn net.Conn) Conn {
	c := &conn{
		nextConn: netConn,
		closed:   make(chan struct{}),
	}

	return c
}

// ReadContext reads data from the connection.
// Unlike net.Conn.Read(), the provided context is used to control timeout.
func (c *conn) ReadContext(ctx context.Context, b []byte) (int, error) { //nolint:cyclop
	c.readMu.Lock()
	defer c.readMu.Unlock()

	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}

	finish := cancelOnDone(ctx, c.nextConn.SetReadDeadline)
	n, err := c.nextConn.Read(b)
	errSetDeadline := finish()
	if e := ctx.Err(); e != nil && n == 0 {
		err = e
	}
	if err == nil && errSetDeadline != nil {
		err = errSetDeadline
	}

	return n, err
}

// WriteContext writes data to the connection.
// Unlike net.Conn.Write(), the provided context is used to control timeout.
func (c *conn) WriteContext(ctx context.Context, b []byte) (int, error) { //nolint:cyclop
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return 0, ErrClosing
	default:
	}

	finish := cancelOnDone(ctx, c.nextConn.SetWriteDeadline)
	n, err := c.nextConn.Write(b)
	errSetDeadline := finish()
	if e := ctx.Err(); e != nil && n == 0 {
		err = e
	}
	if err == nil && errSetDeadline != nil {
		err = errSetDeadline
	}

	return n, err
}

// Close closes the connection.
// Any blocked ReadContext or WriteContext operations will be unblocked and
// return errors.
func (c *conn) Close() error {
	err := c.nextConn.Close()
	c.closeOnce.Do(func() {
		c.writeMu.Lock()
		c.readMu.Lock()
		close(c.closed)
		c.readMu.Unlock()
		c.writeMu.Unlock()
	})

	return err
}

// LocalAddr returns the local network address, if known.
func (c *conn) LocalAddr() net.Addr {
	return c.nextConn.LocalAddr()
}

// LocalAddr returns the local network address, if known.
func (c *conn) RemoteAddr() net.Addr {
	return c.nextConn.RemoteAddr()
}

// Conn returns the underlying net.Conn.
func (c *conn) Conn() net.Conn {
	return c.nextConn
}
