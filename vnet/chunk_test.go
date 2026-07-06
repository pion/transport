// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"strings"
	"testing"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestChunkUDPClone(t *testing.T) {
	src := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1111}
	dst := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 2222}

	t.Run("AllFieldsCopied", func(t *testing.T) {
		orig := newChunkUDP(src, dst)
		orig.userData = []byte("hello")
		orig.setTimestamp()

		cloned := orig.Clone().(*chunkUDP) //nolint:forcetypeassert

		assert.Equal(t, orig.sourceIP.String(), cloned.sourceIP.String())
		assert.Equal(t, orig.destinationIP.String(), cloned.destinationIP.String())
		assert.Equal(t, orig.sourcePort, cloned.sourcePort)
		assert.Equal(t, orig.destinationPort, cloned.destinationPort)
		assert.Equal(t, orig.tag, cloned.tag)
		assert.Equal(t, orig.timestamp, cloned.timestamp)
		assert.Equal(t, orig.userData, cloned.userData)
	})

	t.Run("UserDataDeepCopy", func(t *testing.T) {
		orig := newChunkUDP(src, dst)
		orig.userData = []byte("hello")

		cloned := orig.Clone().(*chunkUDP) //nolint:forcetypeassert

		orig.userData[0] = 'X'
		assert.Equal(t, byte('h'), cloned.userData[0], "clone should not be affected by mutation of original userData")
	})

	t.Run("NilUserData", func(t *testing.T) {
		orig := newChunkUDP(src, dst)
		// userData is nil by default

		cloned := orig.Clone().(*chunkUDP) //nolint:forcetypeassert

		assert.Nil(t, cloned.userData)
	})
}

func TestChunkTCPClone(t *testing.T) {
	src := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1111}
	dst := &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 2222}

	t.Run("AllFieldsCopied", func(t *testing.T) {
		orig := newChunkTCP(src, dst, tcpSYN|tcpACK)
		orig.userData = []byte("hello")
		orig.seqNum = 42
		orig.ackNum = 99
		orig.duplicate = true
		orig.setTimestamp()

		cloned := orig.Clone().(*chunkTCP) //nolint:forcetypeassert

		assert.Equal(t, orig.sourceIP.String(), cloned.sourceIP.String())
		assert.Equal(t, orig.destinationIP.String(), cloned.destinationIP.String())
		assert.Equal(t, orig.sourcePort, cloned.sourcePort)
		assert.Equal(t, orig.destinationPort, cloned.destinationPort)
		assert.Equal(t, orig.tag, cloned.tag)
		assert.Equal(t, orig.timestamp, cloned.timestamp)
		assert.Equal(t, orig.flags, cloned.flags)
		assert.Equal(t, orig.seqNum, cloned.seqNum)
		assert.Equal(t, orig.ackNum, cloned.ackNum)
		assert.Equal(t, orig.duplicate, cloned.duplicate)
		assert.Equal(t, orig.userData, cloned.userData)
	})

	t.Run("UserDataDeepCopy", func(t *testing.T) {
		orig := newChunkTCP(src, dst, tcpPSH)
		orig.userData = []byte("hello")

		cloned := orig.Clone().(*chunkTCP) //nolint:forcetypeassert

		orig.userData[0] = 'X'
		assert.Equal(t, byte('h'), cloned.userData[0], "clone should not be affected by mutation of original userData")
	})

	t.Run("IndependentAddrs", func(t *testing.T) {
		orig := newChunkTCP(src, dst, tcpSYN)

		cloned := orig.Clone().(*chunkTCP) //nolint:forcetypeassert

		assert.NoError(t, orig.setSourceAddr("9.9.9.9:9999"))
		assert.Equal(t, "10.0.0.1:1111", cloned.SourceAddr().String(), "clone source addr should be unchanged")
		assert.NoError(t, orig.setDestinationAddr("8.8.8.8:8888"))
		assert.Equal(t, "10.0.0.2:2222", cloned.DestinationAddr().String(), "clone destination addr should be unchanged")
	})
}

func TestTCPFragString(t *testing.T) {
	f := tcpFIN
	assert.Equal(t, "FIN", f.String(), "should match")
	f = tcpSYN
	assert.Equal(t, "SYN", f.String(), "should match")
	f = tcpRST
	assert.Equal(t, "RST", f.String(), "should match")
	f = tcpPSH
	assert.Equal(t, "PSH", f.String(), "should match")
	f = tcpACK
	assert.Equal(t, "ACK", f.String(), "should match")
	f = tcpSYN | tcpACK
	assert.Equal(t, "SYN-ACK", f.String(), "should match")
}

func TestChunk(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("ChunkUDP", func(t *testing.T) {
		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP(demoIP),
			Port: 5678,
		}

		var chunk Chunk = newChunkUDP(src, dst)
		str := chunk.String()
		log.Debugf("chunk: %s", str)
		assert.Equal(t, udp, chunk.Network(), "should match")
		assert.True(t, strings.Contains(str, src.Network()), "should include network type")
		assert.True(t, strings.Contains(str, src.String()), "should include address")
		assert.True(t, strings.Contains(str, dst.String()), "should include address")
		assert.True(t, chunk.getSourceIP().Equal(src.IP), "ip should match")
		assert.True(t, chunk.getDestinationIP().Equal(dst.IP), "ip should match")

		// Test timestamp
		ts := chunk.setTimestamp()
		assert.Equal(t, ts, chunk.getTimestamp(), "timestamp should match")

		uc := chunk.(*chunkUDP) //nolint:forcetypeassert
		uc.userData = []byte("Hello")

		cloned := chunk.Clone().(*chunkUDP) //nolint:forcetypeassert

		// Test setSourceAddr
		err := uc.setSourceAddr("2.3.4.5:4000")
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, "2.3.4.5:4000", uc.SourceAddr().String())

		// Test Tag()
		assert.True(t, len(uc.tag) > 0, "should not be empty")
		assert.Equal(t, uc.tag, uc.Tag(), "should match")

		// Verify cloned chunk was not affected by the changes to original chunk
		uc.userData[0] = []byte("!")[0] // original: "Hello" -> "Hell!"
		assert.Equal(t, "Hello", string(cloned.userData), "should match")
		assert.Equal(t, "192.168.0.2:1234", cloned.SourceAddr().String())
		assert.True(t, cloned.getSourceIP().Equal(src.IP), "ip should match")
		assert.True(t, cloned.getDestinationIP().Equal(dst.IP), "ip should match")
	})

	t.Run("ChunkTCP", func(t *testing.T) {
		src := &net.TCPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.TCPAddr{
			IP:   net.ParseIP(demoIP),
			Port: 5678,
		}

		var chunk Chunk = newChunkTCP(src, dst, tcpSYN)
		str := chunk.String()
		log.Debugf("chunk: %s", str)
		assert.Equal(t, tcp, chunk.Network(), "should match")
		assert.True(t, strings.Contains(str, src.Network()), "should include network type")
		assert.True(t, strings.Contains(str, src.String()), "should include address")
		assert.True(t, strings.Contains(str, dst.String()), "should include address")
		assert.True(t, chunk.getSourceIP().Equal(src.IP), "ip should match")
		assert.True(t, chunk.getDestinationIP().Equal(dst.IP), "ip should match")

		tcp, ok := chunk.(*chunkTCP)
		assert.True(t, ok, "type should match")
		assert.Equal(t, tcp.flags, tcpSYN, "flags should match")

		// Test timestamp
		ts := chunk.setTimestamp()
		assert.Equal(t, ts, chunk.getTimestamp(), "timestamp should match")

		tc := chunk.(*chunkTCP) //nolint:forcetypeassert
		tc.userData = []byte("Hello")

		cloned := chunk.Clone().(*chunkTCP) //nolint:forcetypeassert

		// Test setSourceAddr
		err := tc.setSourceAddr("2.3.4.5:4000")
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, "2.3.4.5:4000", tc.SourceAddr().String())

		// Test Tag()
		assert.True(t, len(tc.tag) > 0, "should not be empty")
		assert.Equal(t, tc.tag, tc.Tag(), "should match")

		// Verify cloned chunk was not affected by the changes to original chunk
		tc.userData[0] = []byte("!")[0] // original: "Hello" -> "Hell!"
		assert.Equal(t, "Hello", string(cloned.userData), "should match")
		assert.Equal(t, "192.168.0.2:1234", cloned.SourceAddr().String())
		assert.True(t, cloned.getSourceIP().Equal(src.IP), "ip should match")
		assert.True(t, cloned.getDestinationIP().Equal(dst.IP), "ip should match")

		// Test setDestinationAddr
		err = tc.setDestinationAddr("3.4.5.6:7000")
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, "3.4.5.6:7000", tc.DestinationAddr().String())
	})
}
