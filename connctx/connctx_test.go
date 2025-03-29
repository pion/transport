// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package connctx

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRead(t *testing.T) {
	ca, cb := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	data := []byte{0x01, 0x02, 0xFF}
	chErr := make(chan error)

	go func() {
		_, err := cb.Write(data)
		chErr <- err
	}()

	c := New(ca)
	b := make([]byte, 100)
	n, err := c.ReadContext(context.Background(), b)
	assert.NoError(t, err)
	assert.Len(t, data, n)
	assert.Equal(t, data, b[:n])

	err = <-chErr
	assert.NoError(t, err)
}

func TestReadTimeout(t *testing.T) {
	ca, _ := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := New(ca)
	b := make([]byte, 100)
	n, err := c.ReadContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n)
}

func TestReadCancel(t *testing.T) {
	ca, _ := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	c := New(ca)
	b := make([]byte, 100)
	n, err := c.ReadContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n)
}

func TestReadClosed(t *testing.T) {
	ca, _ := net.Pipe()

	c := New(ca)
	_ = c.Close()

	b := make([]byte, 100)
	n, err := c.ReadContext(context.Background(), b)
	assert.ErrorIs(t, err, io.EOF)
	assert.Empty(t, n)
}

func TestWrite(t *testing.T) {
	ca, cb := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	chErr := make(chan error)
	chRead := make(chan []byte)

	go func() {
		b := make([]byte, 100)
		n, err := cb.Read(b)
		chErr <- err
		chRead <- b[:n]
	}()

	c := New(ca)
	data := []byte{0x01, 0x02, 0xFF}
	n, err := c.WriteContext(context.Background(), data)
	assert.NoError(t, err)
	assert.Len(t, data, n)

	err = <-chErr
	b := <-chRead
	assert.Equal(t, b, data)
	assert.NoError(t, err)
}

func TestWriteTimeout(t *testing.T) {
	ca, _ := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	c := New(ca)
	b := make([]byte, 100)
	n, err := c.WriteContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n)
}

func TestWriteCancel(t *testing.T) {
	ca, _ := net.Pipe()
	defer func() {
		_ = ca.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	c := New(ca)
	b := make([]byte, 100)
	n, err := c.WriteContext(ctx, b)
	assert.Error(t, err)
	assert.Empty(t, n)
}

func TestWriteClosed(t *testing.T) {
	ca, _ := net.Pipe()

	c := New(ca)
	_ = c.Close()

	b := make([]byte, 100)
	n, err := c.WriteContext(context.Background(), b)
	assert.ErrorIs(t, err, ErrClosing)
	assert.Empty(t, n)
}

// Test for TestLocalAddrAndRemoteAddr.
type stringAddr struct {
	network string
	addr    string
}

func (a stringAddr) Network() string { return a.network }
func (a stringAddr) String() string  { return a.addr }

type connAddrMock struct{}

func (*connAddrMock) RemoteAddr() net.Addr { return stringAddr{"remote_net", "remote_addr"} }
func (*connAddrMock) LocalAddr() net.Addr  { return stringAddr{"local_net", "local_addr"} }
func (*connAddrMock) Read(_ []byte) (n int, err error) {
	panic("unimplemented") //nolint
}

func (*connAddrMock) Write(_ []byte) (n int, err error) {
	panic("unimplemented") //nolint
}

func (*connAddrMock) Close() error {
	panic("unimplemented") //nolint
}

func (*connAddrMock) SetDeadline(_ time.Time) error {
	panic("unimplemented") //nolint
}

func (*connAddrMock) SetReadDeadline(_ time.Time) error {
	panic("unimplemented") //nolint
}

func (*connAddrMock) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented") //nolint
}

func TestLocalAddrAndRemoteAddr(t *testing.T) {
	c := New(&connAddrMock{})
	al := c.LocalAddr()
	ar := c.RemoteAddr()

	assert.Equal(t, "local_addr", al.String())
	assert.Equal(t, "remote_addr", ar.String())
}

func BenchmarkBase(b *testing.B) {
	ca, cb := net.Pipe()
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
		for i := 0; i < n; i++ {
			_, _ = cb.Write(data)
		}
		_ = cb.Close()
	}(b.N)

	count := 0
	for {
		n, err := ca.Read(buf)
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

func BenchmarkWrite(b *testing.B) {
	ca, cb := net.Pipe()
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
		c := New(cb)
		for i := 0; i < n; i++ {
			_, _ = c.WriteContext(context.Background(), data)
		}
		_ = cb.Close()
	}(b.N)

	count := 0
	for {
		n, err := ca.Read(buf)
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

func BenchmarkRead(b *testing.B) {
	ca, cb := net.Pipe()
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
		for i := 0; i < n; i++ {
			_, _ = cb.Write(data)
		}
		_ = cb.Close()
	}(b.N)

	c := New(ca)
	count := 0
	for {
		n, err := c.ReadContext(context.Background(), buf)
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
