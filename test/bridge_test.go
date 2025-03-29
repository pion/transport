// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
)

const (
	msg1 = `ADC`
	msg2 = `DEFG`
)

// helper to close both conns.
func closeBridge(br *Bridge) error {
	if err := br.conn0.Close(); err != nil {
		return err
	}

	return br.conn1.Close()
}

type AsyncResult struct {
	n   int
	err error
	msg string
}

func TestBridge(t *testing.T) { //nolint:gocyclo,cyclop,maintidx
	tt := TimeOut(30 * time.Second)
	defer tt.Stop()

	buf := make([]byte, 256)

	t.Run("normal", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		assert.Equal(t, "a0", conn0.LocalAddr().String())
		assert.Equal(t, "a1", conn1.LocalAddr().String())
		assert.Equal(t, "udp", conn0.LocalAddr().Network())
		assert.Equal(t, "udp", conn1.LocalAddr().Network())

		n, err := conn0.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")

		go func() {
			nInner, errInner := conn1.Read(buf)
			readRes <- AsyncResult{n: nInner, err: errInner}
		}()

		br.Process()

		ar := <-readRes
		assert.NoError(t, ar.err)
		assert.Len(t, msg1, ar.n, "unexpected length")
		assert.NoError(t, closeBridge(br))
	})

	t.Run("drop 1st packet from conn0", func(t *testing.T) { //nolint:dupl
		readRes := make(chan AsyncResult)
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn0.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn0.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		go func() {
			nInner, errInner := conn1.Read(buf)
			readRes <- AsyncResult{n: nInner, err: errInner}
		}()

		br.Drop(0, 0, 1)
		br.Process()

		ar := <-readRes
		assert.NoError(t, ar.err)
		assert.Len(t, msg2, ar.n, "unexpected length")
		assert.NoError(t, closeBridge(br))
	})

	t.Run("drop 2nd packet from conn0", func(t *testing.T) { //nolint:dupl
		readRes := make(chan AsyncResult)
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn0.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn0.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		go func() {
			nInner, errInner := conn1.Read(buf)
			readRes <- AsyncResult{n: nInner, err: errInner}
		}()

		br.Drop(0, 1, 1)
		br.Process()

		ar := <-readRes
		assert.NoError(t, ar.err)
		assert.Len(t, msg1, ar.n, "unexpected length")
		assert.NoError(t, closeBridge(br))
	})

	t.Run("drop 1st packet from conn1", func(t *testing.T) { //nolint:dupl
		readRes := make(chan AsyncResult)
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn1.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn1.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		go func() {
			nInner, errInner := conn0.Read(buf)
			readRes <- AsyncResult{n: nInner, err: errInner}
		}()

		br.Drop(1, 0, 1)
		br.Process()

		ar := <-readRes
		assert.NoError(t, ar.err)
		assert.Len(t, msg2, ar.n, "unexpected length")
		assert.NoError(t, closeBridge(br))
	})

	t.Run("drop 2nd packet from conn1", func(t *testing.T) { //nolint:dupl
		readRes := make(chan AsyncResult)
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn1.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn1.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		go func() {
			nInner, errInner := conn0.Read(buf)
			readRes <- AsyncResult{n: nInner, err: errInner}
		}()

		br.Drop(1, 1, 1)
		br.Process()

		ar := <-readRes
		assert.NoError(t, ar.err)
		assert.Len(t, msg1, ar.n, "unexpected length")
		assert.NoError(t, closeBridge(br))
	})

	t.Run("reorder packets from conn0", func(t *testing.T) { //nolint:dupl
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn0.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn0.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		done := make(chan bool)

		go func() {
			nInner, errInner := conn1.Read(buf)
			assert.NoError(t, errInner)
			assert.Len(t, msg2, nInner, "unexpected length")
			nInner, errInner = conn1.Read(buf)
			assert.NoError(t, errInner)
			assert.Len(t, msg1, nInner, "unexpected length")
			done <- true
		}()

		err = br.Reorder(0)
		assert.NoError(t, err)
		br.Process()
		<-done
		assert.NoError(t, closeBridge(br))
	})

	t.Run("reorder packets from conn1", func(t *testing.T) { //nolint:dupl
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err := conn1.Write([]byte(msg1))
		assert.NoError(t, err)
		assert.Len(t, msg1, n, "unexpected length")
		n, err = conn1.Write([]byte(msg2))
		assert.NoError(t, err)
		assert.Len(t, msg2, n, "unexpected length")

		done := make(chan bool)

		go func() {
			nInner, errInner := conn0.Read(buf)
			assert.NoError(t, errInner)
			assert.Len(t, msg2, nInner, "unexpected length")
			nInner, errInner = conn0.Read(buf)
			assert.NoError(t, errInner)
			assert.Len(t, msg1, nInner, "unexpected length")
			done <- true
		}()

		err = br.Reorder(1)
		assert.NoError(t, err)
		br.Process()
		<-done
		assert.NoError(t, closeBridge(br))
	})

	t.Run("inverse error", func(t *testing.T) {
		q := [][]byte{}
		q = append(q, []byte("ABC"))
		assert.Error(t, inverse(q), "inverse should fail if less than 2 pkts")
	})

	t.Run("drop next N packets", func(t *testing.T) {
		testFrom := func(t *testing.T, fromID int) {
			t.Helper()
			readRes := make(chan AsyncResult, 5)
			br := NewBridge()
			conn0 := br.GetConn0()
			conn1 := br.GetConn1()

			var wg sync.WaitGroup
			wg.Add(1)

			var srcConn, dstConn net.Conn

			if fromID == 0 {
				br.DropNextNWrites(0, 3)
				srcConn = conn0
				dstConn = conn1
			} else {
				br.DropNextNWrites(1, 3)
				srcConn = conn1
				dstConn = conn0
			}

			go func() {
				defer wg.Done()
				for {
					nInner, errInner := dstConn.Read(buf)
					if errInner != nil {
						break
					}
					readRes <- AsyncResult{
						n:   nInner,
						err: nil,
						msg: string(buf),
					}
				}
			}()
			msgs := make([]string, 0)

			for i := 0; i < 5; i++ {
				msg := fmt.Sprintf("msg%d", i)
				msgs = append(msgs, msg)
				n, err := srcConn.Write([]byte(msg))
				assert.NoErrorf(t, err, "Test: %d", fromID)
				assert.Len(t, msg, n, "[%d] unexpected length", fromID)

				br.Process()
			}

			assert.NoErrorf(t, closeBridge(br), "Test: %d", fromID)
			br.Process()
			wg.Wait()

			assert.Lenf(t, readRes, 2, "[%d] unexpected number of packets", fromID)

			for i := 0; i < 2; i++ {
				ar := <-readRes
				assert.NoErrorf(t, ar.err, "Test: %d", fromID)

				assert.Len(t, msgs[i+3], ar.n, "[%d] unexpected length", fromID)
			}
		}

		testFrom(t, 0)
		testFrom(t, 1)
	})
}

type closePropagator struct {
	*bridgeConn
	otherEnd *bridgeConn
}

func (c *closePropagator) Close() error {
	c.otherEnd.mutex.Lock()
	c.otherEnd.closing = true
	c.otherEnd.mutex.Unlock()

	return c.bridgeConn.Close()
}

func TestNetTest(t *testing.T) {
	nettest.TestConn(t, func() (net.Conn, net.Conn, func(), error) {
		br := NewBridge()
		conn0 := br.GetConn0().(*bridgeConn) //nolint:forcetypeassert
		conn1 := br.GetConn1().(*bridgeConn) //nolint:forcetypeassert
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			for {
				br.Process()
				if conn0.isClosed() && conn1.isClosed() {
					wg.Done()

					return
				}
			}
		}()

		return &closePropagator{conn0, conn1}, &closePropagator{conn1, conn0},
			func() {
				// RacyRead test leave receive buffer filled.
				// As net.Conn.Read() should return received data even after Close()-ed,
				// queue must be cleared explicitly.
				br.clear()
				_ = conn0.Close()
				_ = conn1.Close()

				// Tick the clock to actually close conns.
				br.Tick()

				wg.Wait()
			}, nil
	})
}
