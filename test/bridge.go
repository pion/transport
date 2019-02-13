package test

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// bridgeConn is a net.Conn that represents an endpoint of the bridge.
type bridgeConn struct {
	br     *Bridge
	id     int
	readCh chan []byte
}

// Read reads data, block until the data becomes available.
func (conn *bridgeConn) Read(b []byte) (int, error) {
	if data, ok := <-conn.readCh; ok {
		n := copy(b, data)
		return n, nil
	}
	return 0, fmt.Errorf("bridgeConn closed")
}

// Write writes data to the bridge.
func (conn *bridgeConn) Write(b []byte) (int, error) {
	n := len(b)
	conn.br.Push(b, conn.id)
	return n, nil
}

// Close closes the bridge (releases resources used).
func (conn *bridgeConn) Close() error {
	close(conn.readCh)
	return nil
}

// LocalAddr is not used
func (conn *bridgeConn) LocalAddr() net.Addr { return nil }

// RemoteAddr is not used
func (conn *bridgeConn) RemoteAddr() net.Addr { return nil }

// SetDeadline is not used
func (conn *bridgeConn) SetDeadline(t time.Time) error { return nil }

// SetReadDeadline is not used
func (conn *bridgeConn) SetReadDeadline(t time.Time) error { return nil }

// SetWriteDeadline is not used
func (conn *bridgeConn) SetWriteDeadline(t time.Time) error { return nil }

// Bridge represents a network between the two endpoints.
type Bridge struct {
	mutex sync.RWMutex
	conn0 *bridgeConn
	conn1 *bridgeConn

	queue0to1 [][]byte
	queue1to0 [][]byte
}

func inverse(s [][]byte) error {
	if len(s) < 2 {
		return fmt.Errorf("inverse requires more than one item in the array")
	}

	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return nil
}

// drop n packets from the slice starting from offset
func drop(s [][]byte, offset, n int) [][]byte {
	if offset+n > len(s) {
		n = len(s) - offset
	}
	return append(s[:offset], s[offset+n:]...)
}

func NewBridge() (*Bridge, net.Conn, net.Conn) {
	br := &Bridge{
		queue0to1: make([][]byte, 0),
		queue1to0: make([][]byte, 0),
	}

	br.conn0 = &bridgeConn{
		br:     br,
		id:     0,
		readCh: make(chan []byte),
	}
	br.conn1 = &bridgeConn{
		br:     br,
		id:     1,
		readCh: make(chan []byte),
	}

	return br, br.conn0, br.conn1
}

func (br *Bridge) Push(d []byte, fromID int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.queue0to1 = append(br.queue0to1, d)
	} else {
		br.queue1to0 = append(br.queue1to0, d)
	}
}

func (br *Bridge) Reorder(fromID int) error {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	var err error

	if fromID == 0 {
		err = inverse(br.queue0to1)
	} else {
		err = inverse(br.queue1to0)
	}

	return err
}

func (br *Bridge) Drop(fromID, offset, n int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.queue0to1 = drop(br.queue0to1, offset, n)
	} else {
		br.queue1to0 = drop(br.queue1to0, offset, n)
	}
}

func (br *Bridge) Tick() int {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	var n int

	if len(br.queue0to1) > 0 {
		select {
		case br.conn1.readCh <- br.queue0to1[0]:
			n++
			//fmt.Printf("conn1 received data (%d bytes)\n", len(br.queue0to1[0]))
			br.queue0to1 = br.queue0to1[1:]
		default:
		}
	}

	if len(br.queue1to0) > 0 {
		select {
		case br.conn0.readCh <- br.queue1to0[0]:
			n++
			//fmt.Printf("conn0 received data (%d bytes)\n", len(br.queue1to0[0]))
			br.queue1to0 = br.queue1to0[1:]
		default:
		}
	}

	return n
}

// Repeat tick() call until no more outstanding inflight packet
func (br *Bridge) Process() {
	for {
		time.Sleep(10 * time.Millisecond)
		n := br.Tick()
		if n == 0 {
			break
		}
	}
}
