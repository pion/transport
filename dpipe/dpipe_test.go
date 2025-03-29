// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package dpipe

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
)

var errFailedToCast = fmt.Errorf("failed to cast net.Conn to conn")

func TestNetTest(t *testing.T) {
	nettest.TestConn(t, func() (net.Conn, net.Conn, func(), error) {
		ca, cb := Pipe()
		caConn, ok := ca.(*conn)
		if !ok {
			return nil, nil, nil, errFailedToCast
		}

		cbConn, ok := cb.(*conn)
		if !ok {
			return nil, nil, nil, errFailedToCast
		}

		return &closePropagator{caConn, cbConn},
			&closePropagator{cbConn, caConn},
			func() {
				_ = ca.Close()
				_ = cb.Close()
			}, nil
	})
}

type closePropagator struct {
	*conn
	otherEnd *conn
}

func (c *closePropagator) Close() error {
	close(c.otherEnd.closing)

	return c.conn.Close()
}

func TestPipe(t *testing.T) { //nolint:cyclop
	ca, cb := Pipe()

	testData := []byte{0x01, 0x02}

	for name, cond := range map[string]struct {
		ca net.Conn
		cb net.Conn
	}{
		"AtoB": {ca, cb},
		"BtoA": {cb, ca},
	} {
		c0 := cond.ca
		c1 := cond.cb
		t.Run(name, func(t *testing.T) {
			n, err := c0.Write(testData)
			assert.NoError(t, err)
			assert.Equal(t, len(testData), n)

			readData := make([]byte, 4)
			n, err = c1.Read(readData)
			assert.NoError(t, err)
			assert.Len(t, testData, n)
			assert.Equal(t, testData, readData[:n])
		})
	}

	assert.NoError(t, ca.Close())
	_, err := ca.Write(testData)
	assert.ErrorIs(t, err, io.ErrClosedPipe, "Write to closed conn should fail")

	// Other side should be writable.
	_, err = cb.Write(testData)
	assert.NoError(t, err)

	readData := make([]byte, 4)
	_, err = ca.Read(readData)
	assert.ErrorIs(t, err, io.EOF, "Read from closed conn should fail with io.EOF")

	// Other side should be readable.
	readDone := make(chan struct{})
	go func() {
		readData := make([]byte, 4)
		n, err := cb.Read(readData)
		assert.Errorf(t, err, "Unexpected data %v was arrived to orphaned conn", readData[:n])
		close(readDone)
	}()
	select {
	case <-readDone:
		assert.Fail(t, "Read should be blocked if the other side is closed")
	case <-time.After(10 * time.Millisecond):
	}
	assert.NoError(t, cb.Close())
}
