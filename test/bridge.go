package test

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

type bridgeConnAddr int

func (a bridgeConnAddr) Network() string {
	return "udp"
}
func (a bridgeConnAddr) String() string {
	return fmt.Sprintf("a%d", a)
}

// bridgeConn is a net.Conn that represents an endpoint of the bridge.
type bridgeConn struct {
	br         *Bridge
	id         int
	closed     bool
	mutex      sync.RWMutex
	readCh     chan []byte
	lossChance int
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
	if rand.Intn(100) < conn.lossChance {
		return len(b), nil
	}

	conn.br.Push(b, conn.id)
	return len(b), nil
}

// Close closes the bridge (releases resources used).
func (conn *bridgeConn) Close() error {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.closed {
		return fmt.Errorf("bridge has already been closed")
	}

	conn.closed = true
	close(conn.readCh)
	return nil
}

// LocalAddr is not used
func (conn *bridgeConn) LocalAddr() net.Addr {
	return bridgeConnAddr(conn.id)
}

// RemoteAddr is not used
func (conn *bridgeConn) RemoteAddr() net.Addr { return nil }

// SetDeadline is not used
func (conn *bridgeConn) SetDeadline(t time.Time) error { return nil }

// SetReadDeadline is not used
func (conn *bridgeConn) SetReadDeadline(t time.Time) error { return nil }

// SetWriteDeadline is not used
func (conn *bridgeConn) SetWriteDeadline(t time.Time) error { return nil }

func (conn *bridgeConn) isClosed() bool {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()

	return conn.closed
}

// Bridge represents a network between the two endpoints.
type Bridge struct {
	mutex sync.RWMutex
	conn0 *bridgeConn
	conn1 *bridgeConn

	queue0to1 [][]byte
	queue1to0 [][]byte

	dropNWrites0    int
	dropNWrites1    int
	reorderNWrites0 int
	reorderNWrites1 int
	stack0          [][]byte
	stack1          [][]byte
	filterCB0       func([]byte) bool
	filterCB1       func([]byte) bool
	err             error // last error
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

// NewBridge creates a new bridge with two endpoints.
func NewBridge() *Bridge {
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

	return br
}

// GetConn0 returns an endpoint of the bridge, conn0.
func (br *Bridge) GetConn0() net.Conn {
	return br.conn0
}

// GetConn1 returns an endpoint of the bridge, conn1.
func (br *Bridge) GetConn1() net.Conn {
	return br.conn1
}

// Push pushes a packet into the specified queue.
func (br *Bridge) Push(d []byte, fromID int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		switch {
		case br.dropNWrites0 > 0:
			br.dropNWrites0--
			//fmt.Printf("br: dropped a packet of size %d (rem: %d for q0)\n", len(d), br.dropNWrites0)
		case br.reorderNWrites0 > 0:
			br.reorderNWrites0--
			br.stack0 = append(br.stack0, d)
			//fmt.Printf("stack0 size: %d\n", len(br.stack0))
			if br.reorderNWrites0 == 0 {
				if err := inverse(br.stack0); err == nil {
					//fmt.Printf("stack0 reordered!\n")
					br.queue0to1 = append(br.queue0to1, br.stack0...)
				} else {
					br.err = err
				}
			}
		case br.filterCB0 != nil && !br.filterCB0(d):
			//fmt.Printf("br: filtered out a packet of size %d (q0)\n", len(d))
		default:
			//fmt.Printf("br: routed a packet of size %d (q0)\n", len(d))
			br.queue0to1 = append(br.queue0to1, d)
		}
	} else {
		switch {
		case br.dropNWrites1 > 0:
			br.dropNWrites1--
			//fmt.Printf("br: dropped a packet of size %d (rem: %d for q1)\n", len(d), br.dropNWrites0)
		case br.reorderNWrites1 > 0:
			br.reorderNWrites1--
			br.stack1 = append(br.stack1, d)
			if br.reorderNWrites1 == 0 {
				if err := inverse(br.stack1); err != nil {
					br.err = err
				}
				br.queue1to0 = append(br.queue1to0, br.stack1...)
			}
		case br.filterCB1 != nil && !br.filterCB1(d):
			//fmt.Printf("br: filtered out a packet of size %d (q1)\n", len(d))
		default:
			//fmt.Printf("br: routed a packet of size %d (q1)\n", len(d))
			br.queue1to0 = append(br.queue1to0, d)
		}
	}
}

// Reorder inverses the order of packets currently in the specified queue.
func (br *Bridge) Reorder(fromID int) error {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		return inverse(br.queue0to1)
	}
	return inverse(br.queue1to0)
}

// Drop drops the specified number of packets from the given offset index
// of the specified queue.
func (br *Bridge) Drop(fromID, offset, n int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.queue0to1 = drop(br.queue0to1, offset, n)
	} else {
		br.queue1to0 = drop(br.queue1to0, offset, n)
	}
}

// DropNextNWrites drops the next n packets that will be written
// to the specified queue.
func (br *Bridge) DropNextNWrites(fromID, n int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.dropNWrites0 = n
	} else {
		br.dropNWrites1 = n
	}
}

// ReorderNextNWrites drops the next n packets that will be written
// to the specified queue.
func (br *Bridge) ReorderNextNWrites(fromID, n int) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.reorderNWrites0 = n
	} else {
		br.reorderNWrites1 = n
	}
}

// Tick attempts to hand a packet from the queue for each directions, to readers,
// if there are waiting on the queue. If there's no reader, it will return
// immediately.
func (br *Bridge) Tick() int {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	var n int

	if len(br.queue0to1) > 0 && !br.conn1.isClosed() {
		select {
		case br.conn1.readCh <- br.queue0to1[0]:
			n++
			//fmt.Printf("conn1 received data (%d bytes)\n", len(br.queue0to1[0]))
			br.queue0to1 = br.queue0to1[1:]
		default:
		}
	}

	if len(br.queue1to0) > 0 && !br.conn0.isClosed() {
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

// Process repeats tick() calls until no more outstanding packet in the queues.
func (br *Bridge) Process() {
	for {
		time.Sleep(10 * time.Millisecond)
		n := br.Tick()
		if n == 0 {
			break
		}
	}
}

// SetLossChance sets the probability of writes being discard (to introduce artificial loss)
func (br *Bridge) SetLossChance(chance int) error {
	if chance > 100 || chance < 0 {
		return fmt.Errorf("loss must be < 100 && > 0")
	}

	rand.Seed(time.Now().UTC().UnixNano())
	br.conn0.lossChance = chance
	br.conn1.lossChance = chance
	return nil
}

// Filter filters (drops) packets based on return value of the given callback.
func (br *Bridge) Filter(fromID int, cb func([]byte) bool) {
	br.mutex.Lock()
	defer br.mutex.Unlock()

	if fromID == 0 {
		br.filterCB0 = cb
	} else {
		br.filterCB1 = cb
	}
}
