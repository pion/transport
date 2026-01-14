// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestTCPListener(ip string, port int) *TCPListener {
	return &TCPListener{
		locAddr: &net.TCPAddr{IP: net.ParseIP(ip), Port: port},
	}
}

func newTestTCPConn(locIP string, locPort int, remIP string, remPort int) *TCPConn {
	return &TCPConn{
		locAddr: &net.TCPAddr{IP: net.ParseIP(locIP), Port: locPort},
		remAddr: &net.TCPAddr{IP: net.ParseIP(remIP), Port: remPort},
	}
}

func findTCPConnByTuple(m *tcpConnMap, dstIP string, dstPort int, srcIP string, srcPort int) (*TCPConn, bool) {
	c := newChunkTCP(
		&net.TCPAddr{IP: net.ParseIP(srcIP), Port: srcPort},
		&net.TCPAddr{IP: net.ParseIP(dstIP), Port: dstPort},
		tcpACK,
	)

	return m.findByChunk(c)
}

func TestTCPListenerMap(t *testing.T) {
	t.Run("insert a TCPListener and remove it", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("127.0.0.1", 1234)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		out, ok := listenerMap.find(l1.Addr().(*net.TCPAddr)) //nolint:forcetypeassert
		assert.True(t, ok, "should succeed")
		assert.Equal(t, l1, out, "should match")
		assert.Equal(t, 1, len(listenerMap.portMap), "should match")

		err = listenerMap.delete(l1.Addr())
		assert.NoError(t, err, "should succeed")
		assert.Empty(t, listenerMap.portMap, "should match")

		err = listenerMap.delete(l1.Addr())
		assert.Error(t, err, "should fail")
	})

	t.Run("insert a TCPListener on 0.0.0.0 and remove it", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("0.0.0.0", 1234)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		out, ok := listenerMap.find(l1.Addr().(*net.TCPAddr)) //nolint:forcetypeassert
		assert.True(t, ok, "should succeed")
		assert.Equal(t, l1, out, "should match")
		assert.Equal(t, 1, len(listenerMap.portMap), "should match")

		err = listenerMap.delete(l1.Addr())
		assert.NoError(t, err, "should succeed")

		err = listenerMap.delete(l1.Addr())
		assert.Error(t, err, "should fail")
	})

	t.Run("find TCPListener on 0.0.0.0 by specified IP", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("0.0.0.0", 1234)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		out, ok := listenerMap.find(&net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 1234})
		assert.True(t, ok, "should succeed")
		assert.Equal(t, l1, out, "should match")
		assert.Equal(t, 1, len(listenerMap.portMap), "should match")
	})

	t.Run("insert many IPs with the same port", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("10.1.2.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		l2 := newTestTCPListener("10.1.2.2", 5678)
		err = listenerMap.insert(l2)
		assert.NoError(t, err, "should succeed")

		out1, ok := listenerMap.find(&net.TCPAddr{IP: net.ParseIP("10.1.2.1"), Port: 5678})
		assert.True(t, ok, "should succeed")
		assert.Equal(t, l1, out1, "should match")

		out2, ok := listenerMap.find(&net.TCPAddr{IP: net.ParseIP("10.1.2.2"), Port: 5678})
		assert.True(t, ok, "should succeed")
		assert.Equal(t, l2, out2, "should match")

		assert.Equal(t, 1, len(listenerMap.portMap), "should match")
	})

	t.Run("already in-use when inserting 0.0.0.0", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("10.1.2.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		l2 := newTestTCPListener("0.0.0.0", 5678)
		err = listenerMap.insert(l2)
		assert.Error(t, err, "should fail")
	})

	t.Run("already in-use when inserting a specified IP", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("0.0.0.0", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		l2 := newTestTCPListener("192.168.0.1", 5678)
		err = listenerMap.insert(l2)
		assert.Error(t, err, "should fail")
	})

	t.Run("already in-use when inserting the same specified IP", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("192.168.0.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		l2 := newTestTCPListener("192.168.0.1", 5678)
		err = listenerMap.insert(l2)
		assert.Error(t, err, "should fail")
	})

	t.Run("find failure 1", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("192.168.0.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		_, ok := listenerMap.find(&net.TCPAddr{IP: net.ParseIP("192.168.0.2"), Port: 5678})
		assert.False(t, ok, "should fail")
	})

	t.Run("find failure 2", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("192.168.0.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		_, ok := listenerMap.find(&net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 1234})
		assert.False(t, ok, "should fail")
	})

	t.Run("insert two TCPListeners on the same port, then remove them", func(t *testing.T) {
		listenerMap := newTCPListenerMap()

		l1 := newTestTCPListener("192.168.0.1", 5678)
		err := listenerMap.insert(l1)
		assert.NoError(t, err, "should succeed")

		l2 := newTestTCPListener("192.168.0.2", 5678)
		err = listenerMap.insert(l2)
		assert.NoError(t, err, "should succeed")

		err = listenerMap.delete(l1.Addr())
		assert.NoError(t, err, "should succeed")

		err = listenerMap.delete(l2.Addr())
		assert.NoError(t, err, "should succeed")
	})
}

func TestTCPConnMap(t *testing.T) {
	t.Run("insert a TCPConn and remove it", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("127.0.0.1", 1234, "127.0.0.1", 5678)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		out, ok := findTCPConnByTuple(connMap, "127.0.0.1", 1234, "127.0.0.1", 5678)
		assert.True(t, ok, "should succeed")
		assert.Equal(t, c1, out, "should match")
		assert.Equal(t, 1, len(connMap.portMap), "should match")

		err = connMap.deleteConn(c1)
		assert.NoError(t, err, "should succeed")
		assert.Empty(t, connMap.portMap, "should match")

		err = connMap.deleteConn(c1)
		assert.Error(t, err, "should fail")
	})

	t.Run("insert a TCPConn on 0.0.0.0 and find it by specified IP", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("0.0.0.0", 1234, "10.0.0.2", 5678)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		out, ok := findTCPConnByTuple(connMap, "192.168.0.1", 1234, "10.0.0.2", 5678)
		assert.True(t, ok, "should succeed")
		assert.Equal(t, c1, out, "should match")
	})

	t.Run("insert many remote tuples with the same local port", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("10.1.2.1", 5678, "10.1.2.100", 1111)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		c2 := newTestTCPConn("10.1.2.1", 5678, "10.1.2.101", 2222)
		err = connMap.insert(c2)
		assert.NoError(t, err, "should succeed")

		out1, ok := findTCPConnByTuple(connMap, "10.1.2.1", 5678, "10.1.2.100", 1111)
		assert.True(t, ok, "should succeed")
		assert.Equal(t, c1, out1, "should match")

		out2, ok := findTCPConnByTuple(connMap, "10.1.2.1", 5678, "10.1.2.101", 2222)
		assert.True(t, ok, "should succeed")
		assert.Equal(t, c2, out2, "should match")

		assert.Equal(t, 1, len(connMap.portMap), "should match")
	})

	t.Run("already in-use when inserting the same tuple", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("192.168.0.1", 5678, "192.168.0.2", 9999)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		c2 := newTestTCPConn("192.168.0.1", 5678, "192.168.0.2", 9999)
		err = connMap.insert(c2)
		assert.Error(t, err, "should fail")
	})

	t.Run("find failure 1 (remote mismatch)", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("192.168.0.1", 5678, "192.168.0.2", 9999)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		_, ok := findTCPConnByTuple(connMap, "192.168.0.1", 5678, "192.168.0.3", 9999)
		assert.False(t, ok, "should fail")
	})

	t.Run("find failure 2 (port mismatch)", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("192.168.0.1", 5678, "192.168.0.2", 9999)
		err := connMap.insert(c1)
		assert.NoError(t, err, "should succeed")

		_, ok := findTCPConnByTuple(connMap, "192.168.0.1", 1234, "192.168.0.2", 9999)
		assert.False(t, ok, "should fail")
	})

	t.Run("deleteByAddr removes only matching local IP", func(t *testing.T) {
		connMap := newTCPConnMap()

		c1 := newTestTCPConn("192.168.0.1", 5678, "192.168.0.2", 1111)
		c2 := newTestTCPConn("192.168.0.2", 5678, "192.168.0.3", 2222)

		assert.NoError(t, connMap.insert(c1))
		assert.NoError(t, connMap.insert(c2))

		assert.NoError(t, connMap.deleteByAddr(&net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 5678}))

		_, ok1 := findTCPConnByTuple(connMap, "192.168.0.1", 5678, "192.168.0.2", 1111)
		assert.False(t, ok1, "c1 should be removed")

		out2, ok2 := findTCPConnByTuple(connMap, "192.168.0.2", 5678, "192.168.0.3", 2222)
		assert.True(t, ok2, "c2 should remain")
		assert.Equal(t, c2, out2, "should match")
	})
}
