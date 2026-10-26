// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netctx

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	readOperation  = "read"
	writeOperation = "write"
)

var _ net.PacketConn = wrapConn{}

type wrapConn struct {
	c net.Conn
}

func (w wrapConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, err := w.c.Read(p)

	return n, nil, err
}

func (w wrapConn) WriteTo(p []byte, _ net.Addr) (n int, err error) {
	return w.c.Write(p)
}

func (w wrapConn) Close() error {
	return w.c.Close()
}

func (w wrapConn) LocalAddr() net.Addr {
	return w.c.LocalAddr()
}

func (w wrapConn) RemoteAddr() net.Addr {
	return w.c.RemoteAddr()
}

func (w wrapConn) SetDeadline(t time.Time) error {
	return w.c.SetDeadline(t)
}

func (w wrapConn) SetReadDeadline(t time.Time) error {
	return w.c.SetReadDeadline(t)
}

func (w wrapConn) SetWriteDeadline(t time.Time) error {
	return w.c.SetWriteDeadline(t)
}

func pipe() (net.PacketConn, net.PacketConn) {
	a, b := net.Pipe()

	return wrapConn{a}, wrapConn{b}
}

func TestReadFrom(t *testing.T) {
	ca, cb := pipe()
	defer func() {
		_ = ca.Close()
	}()

	data := []byte{0x01, 0x02, 0xFF}
	chErr := make(chan error)

	go func() {
		_, err := cb.WriteTo(data, nil)
		chErr <- err
	}()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(context.Background(), b)
	assert.NoError(t, err)
	assert.Len(t, data, n, "Wrong data length")
	assert.Equal(t, data, b[:n])
	assert.NoError(t, <-chErr)
}

func TestReadFromTimeout(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestReadFromAlreadyTimeout(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestReadFromCancel(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestReadFromClosed(t *testing.T) {
	ca, _ := pipe()

	c := NewPacketConn(ca)
	_ = c.Close()

	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(context.Background(), b)
	assert.ErrorIs(t, err, net.ErrClosed)
	assert.Empty(t, n, "Wrong data length")
}

func TestReadFromClosedAfter(t *testing.T) {
	ca, _ := pipe()

	c := NewPacketConn(ca)
	go func() {
		<-time.After(time.Second)
		_ = c.Close()
	}()

	b := make([]byte, 100)
	n, _, err := c.ReadFromContext(context.Background(), b)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Empty(t, n, "Wrong data length")
}

var errSetDeadline = errors.New("set read deadline failed")

type badDeadlineConn struct {
	net.PacketConn
}

func (c *badDeadlineConn) SetWriteDeadline(time.Time) error {
	return errSetDeadline
}

func (c *badDeadlineConn) SetReadDeadline(time.Time) error {
	return errSetDeadline
}

func TestReadFromSetDeadlineErr(t *testing.T) {
	ca, _ := pipe()

	b := NewPacketConn(&badDeadlineConn{
		PacketConn: ca,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(time.Second)
		cancel()
	}()

	packet := make([]byte, 100)
	_, _, err := b.ReadFromContext(ctx, packet)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWriteTo(t *testing.T) {
	ca, cb := pipe()
	defer func() {
		_ = ca.Close()
	}()

	chErr := make(chan error)
	chRead := make(chan []byte)

	go func() {
		b := make([]byte, 100)
		n, _, err := cb.ReadFrom(b)
		chErr <- err
		chRead <- b[:n]
	}()

	c := NewPacketConn(ca)
	data := []byte{0x01, 0x02, 0xFF}
	n, err := c.WriteToContext(context.Background(), data, nil)
	assert.NoError(t, err)
	assert.Len(t, data, n, "Wrong data length")

	err = <-chErr
	b := <-chRead
	assert.NoError(t, err)
	assert.Equal(t, data, b)
}

func TestWriteToTimeout(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, err := c.WriteToContext(ctx, b, nil)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestWriteToAlreadyTimeout(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, err := c.WriteToContext(ctx, b, nil)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestWriteToCancel(t *testing.T) {
	ca, _ := pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	c := NewPacketConn(ca)
	b := make([]byte, 100)
	n, err := c.WriteToContext(ctx, b, nil)
	assert.Error(t, err)
	assert.Empty(t, n, "Wrong data length")
}

func TestWriteToClosed(t *testing.T) {
	ca, _ := pipe()

	c := NewPacketConn(ca)
	_ = c.Close()

	b := make([]byte, 100)
	n, err := c.WriteToContext(context.Background(), b, nil)
	assert.ErrorIs(t, err, ErrClosing)
	assert.Empty(t, n, "Wrong data length")
}

func TestWriteToClosedAfter(t *testing.T) {
	ca, _ := pipe()

	c := NewPacketConn(ca)
	go func() {
		<-time.After(time.Second)
		_ = c.Close()
	}()

	b := make([]byte, 100)
	n, err := c.WriteToContext(context.Background(), b, nil)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Empty(t, n, "Wrong data length")
}

func TestWriteToSetDeadlineErr(t *testing.T) {
	ca, _ := pipe()

	b := NewPacketConn(&badDeadlineConn{
		PacketConn: ca,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(time.Second)
		cancel()
	}()

	packet := make([]byte, 100)
	_, err := b.WriteToContext(ctx, packet, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

type packetConnAddrMock struct{}

func (*packetConnAddrMock) LocalAddr() net.Addr                    { return stringAddr{"local_net", "local_addr"} }
func (*packetConnAddrMock) ReadFrom([]byte) (int, net.Addr, error) { panic("unimplemented") } //nolint:forbidigo
func (*packetConnAddrMock) WriteTo([]byte, net.Addr) (int, error)  { panic("unimplemented") } //nolint:forbidigo
func (*packetConnAddrMock) Close() error                           { panic("unimplemented") } //nolint:forbidigo
func (*packetConnAddrMock) SetDeadline(_ time.Time) error          { panic("unimplemented") } //nolint:forbidigo
func (*packetConnAddrMock) SetReadDeadline(_ time.Time) error      { panic("unimplemented") } //nolint:forbidigo
func (*packetConnAddrMock) SetWriteDeadline(_ time.Time) error     { panic("unimplemented") } //nolint:forbidigo

func TestPacketConnLocalAddrAndRemoteAddr(t *testing.T) {
	c := NewPacketConn(&packetConnAddrMock{})
	al := c.LocalAddr()

	assert.Equal(t, "local_addr", al.String())
}

type errorPacketConn struct {
	net.PacketConn
	ioErr       error
	resetErr    error
	deadlines   []time.Time
	cancel      context.CancelFunc
	deadlineSet chan struct{}
	n           int
}

func (c *errorPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	if c.cancel != nil {
		c.cancel()
		<-c.deadlineSet
	}

	return c.n, nil, c.ioErr
}

func (c *errorPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	if c.cancel != nil {
		c.cancel()
		<-c.deadlineSet
	}

	return c.n, c.ioErr
}

func (c *errorPacketConn) SetReadDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	if deadline.IsZero() {
		return c.resetErr
	}
	if c.deadlineSet != nil {
		close(c.deadlineSet)
	}

	return nil
}

func (c *errorPacketConn) SetWriteDeadline(deadline time.Time) error {
	return c.SetReadDeadline(deadline)
}

func TestPacketConnPreservesIOError(t *testing.T) {
	for _, operation := range []string{readOperation, writeOperation} {
		t.Run(operation, func(t *testing.T) {
			for _, ioErr := range []error{io.EOF, &net.OpError{Op: operation, Net: "udp", Err: context.DeadlineExceeded}} {
				t.Run(ioErr.Error(), func(t *testing.T) {
					c := NewPacketConn(&errorPacketConn{ioErr: ioErr})
					var err error
					if operation == readOperation {
						_, _, err = c.ReadFromContext(context.Background(), make([]byte, 1))
					} else {
						_, err = c.WriteToContext(context.Background(), []byte{1}, nil)
					}
					assert.True(t, err == ioErr, "underlying error identity must be preserved") //nolint:err113,errorlint // Test exact identity, not unwrapping.
					if _, ok := ioErr.(net.Error); ok {                                         //nolint:errorlint // Test direct interface compatibility.
						netErr, ok := err.(net.Error) //nolint:errorlint // Test direct interface compatibility.
						if assert.True(t, ok, "error must implement net.Error directly") {
							assert.True(t, netErr.Timeout())
						}
					}
				})
			}
		})
	}
}

func TestPacketConnDeadlineCleanup(t *testing.T) { //nolint:cyclop
	const background = "background"
	for _, operation := range []string{readOperation, writeOperation} {
		for _, mode := range []string{background, "cancelable", "deadline", "canceled during IO"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				cancel := context.CancelFunc(func() {})
				deadline := time.Now().Add(time.Hour)
				if mode == "deadline" {
					ctx, cancel = context.WithDeadline(ctx, deadline)
				} else if mode != background {
					ctx, cancel = context.WithCancel(ctx)
				}
				defer cancel()
				underlying := &errorPacketConn{n: 1}
				if mode == "canceled during IO" {
					underlying.cancel = cancel
					underlying.deadlineSet = make(chan struct{})
				}
				if mode == background || mode == "cancelable" {
					// Model an existing deadline and a connection rejecting resets.
					underlying.deadlines = []time.Time{deadline}
					underlying.resetErr = errSetDeadline
				}
				c := NewPacketConn(underlying)
				var n int
				var err error
				if operation == readOperation {
					n, _, err = c.ReadFromContext(ctx, make([]byte, 1))
				} else {
					n, err = c.WriteToContext(ctx, []byte{1}, nil)
				}
				assert.Equal(t, 1, n)
				assert.NoError(t, err)
				switch mode {
				case background, "cancelable":
					assert.Equal(t, []time.Time{deadline}, underlying.deadlines)
				case "deadline":
					assert.Empty(t, underlying.deadlines)
				default:
					assert.Equal(t, []time.Time{veryOld, {}}, underlying.deadlines)
				}
			})
		}
	}
}

func BenchmarkPacketConnBase(b *testing.B) {
	ca, cb := pipe()
	defer func() {
		_ = ca.Close()
	}()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	buf := make([]byte, len(data))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	go func(n int) {
		for range n {
			_, _ = cb.WriteTo(data, nil)
		}
		_ = cb.Close()
	}(b.N)

	count := 0
	for {
		n, _, err := ca.ReadFrom(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				b.Fatal(err)
			}

			break
		}
		if n != len(data) {
			b.Errorf("Expected %v, got %v", len(data), n)
		}
		count++
	}
	if count != b.N {
		b.Errorf("Expected %v, got %v", b.N, count)
	}
}

func BenchmarkWriteTo(b *testing.B) {
	ca, cb := pipe()
	defer func() {
		_ = ca.Close()
	}()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	buf := make([]byte, len(data))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	go func(n int) {
		c := NewPacketConn(cb)
		for range n {
			_, _ = c.WriteToContext(context.Background(), data, nil)
		}
		_ = cb.Close()
	}(b.N)

	count := 0
	for {
		n, _, err := ca.ReadFrom(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				b.Fatal(err)
			}

			break
		}
		if n != len(data) {
			b.Errorf("Expected %v, got %v", len(data), n)
		}
		count++
	}
	if count != b.N {
		b.Errorf("Expected %v, got %v", b.N, count)
	}
}

func BenchmarkReadFrom(b *testing.B) {
	ca, cb := pipe()
	defer func() {
		_ = ca.Close()
	}()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	buf := make([]byte, len(data))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	go func(n int) {
		for range n {
			_, _ = cb.WriteTo(data, nil)
		}
		_ = cb.Close()
	}(b.N)

	c := NewPacketConn(ca)
	count := 0
	for {
		n, _, err := c.ReadFromContext(context.Background(), buf)
		if n == 0 && err != nil {
			if !errors.Is(err, io.EOF) {
				b.Fatal(err)
			}

			break
		}
		if n != len(data) {
			b.Errorf("Expected %v, got %v", len(data), n)
		}
		count++
	}
	if count != b.N {
		b.Errorf("Expected %v, got %v", b.N, count)
	}
}
