// +build !js

package vnet

import (
	"net"
	"testing"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestNetNative(t *testing.T) {
	log := logging.NewDefaultLoggerFactory().NewLogger("test")

	t.Run("Interfaces", func(t *testing.T) {
		nw := NewNet(nil)
		assert.False(t, nw.IsVirtual(), "should be false")
		interfaces, err := nw.Interfaces()
		assert.NoError(t, err, "should succeed")
		log.Debugf("interfaces: %+v\n", interfaces)
		for _, ifc := range interfaces {
			if ifc.Name == lo0String {
				_, err := ifc.Addrs()
				assert.NoError(t, err, "should succeed")
			}

			if addrs, err := ifc.Addrs(); err == nil {
				for _, addr := range addrs {
					log.Debugf("[%d] %s:%s",
						ifc.Index,
						addr.Network(),
						addr.String())
				}
			}
		}
	})

	t.Run("ResolveUDPAddr", func(t *testing.T) {
		nw := NewNet(nil)

		udpAddr, err := nw.ResolveUDPAddr(udpString, "localhost:1234")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		assert.Contains(t, []string{"127.0.0.1", "127.0.1.1"}, udpAddr.IP.String(), "should match")
		assert.Equal(t, 1234, udpAddr.Port, "should match")
	})

	t.Run("ListenPacket", func(t *testing.T) {
		nw := NewNet(nil)

		conn, err := nw.ListenPacket(udpString, "127.0.0.1:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		udpConn, ok := conn.(*net.UDPConn)
		assert.True(t, ok, "should succeed")
		log.Debugf("udpConn: %+v", udpConn)

		laddr := conn.LocalAddr().String()
		log.Debugf("laddr: %s", laddr)
	})

	t.Run("ListenUDP random port", func(t *testing.T) {
		nw := NewNet(nil)

		srcAddr := &net.UDPAddr{
			IP: net.ParseIP("127.0.0.1"),
		}
		conn, err := nw.ListenUDP(udpString, srcAddr)
		assert.NoError(t, err, "should succeed")

		laddr := conn.LocalAddr().String()
		log.Debugf("laddr: %s", laddr)

		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("Dial (UDP)", func(t *testing.T) {
		nw := NewNet(nil)

		conn, err := nw.Dial(udpString, "127.0.0.1:1234")
		assert.NoError(t, err, "should succeed")

		laddr := conn.LocalAddr()
		log.Debugf("laddr: %s", laddr.String())

		raddr := conn.RemoteAddr()
		log.Debugf("raddr: %s", raddr.String())

		assert.Equal(t, "127.0.0.1", laddr.(*net.UDPAddr).IP.String(), "should match")
		assert.True(t, laddr.(*net.UDPAddr).Port != 0, "should match")
		assert.Equal(t, "127.0.0.1:1234", raddr.String(), "should match")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("DialUDP", func(t *testing.T) {
		nw := NewNet(nil)

		locAddr := &net.UDPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 0,
		}

		remAddr := &net.UDPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 1234,
		}

		conn, err := nw.DialUDP(udpString, locAddr, remAddr)
		assert.NoError(t, err, "should succeed")

		laddr := conn.LocalAddr()
		log.Debugf("laddr: %s", laddr.String())

		raddr := conn.RemoteAddr()
		log.Debugf("raddr: %s", raddr.String())

		assert.Equal(t, "127.0.0.1", laddr.(*net.UDPAddr).IP.String(), "should match")
		assert.True(t, laddr.(*net.UDPAddr).Port != 0, "should match")
		assert.Equal(t, "127.0.0.1:1234", raddr.String(), "should match")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("UDPLoopback", func(t *testing.T) {
		nw := NewNet(nil)

		conn, err := nw.ListenPacket(udpString, "127.0.0.1:0")
		assert.NoError(t, err, "should succeed")
		laddr := conn.LocalAddr()
		msg := "PING!"
		n, err := conn.WriteTo([]byte(msg), laddr)
		assert.NoError(t, err, "should succeed")
		assert.Equal(t, len(msg), n, "should match")

		buf := make([]byte, 1000)
		n, addr, err := conn.ReadFrom(buf)
		assert.NoError(t, err, "should succeed")
		assert.Equal(t, len(msg), n, "should match")
		assert.Equal(t, msg, string(buf[:n]), "should match")
		assert.Equal(t, laddr.(*net.UDPAddr).String(), addr.(*net.UDPAddr).String(), "should match")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("Dialer", func(t *testing.T) {
		nw := NewNet(nil)

		dialer := nw.CreateDialer(&net.Dialer{
			LocalAddr: &net.UDPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 0,
			},
		})

		conn, err := dialer.Dial(udpString, "127.0.0.1:1234")
		assert.NoError(t, err, "should succeed")

		laddr := conn.LocalAddr()
		log.Debugf("laddr: %s", laddr.String())

		raddr := conn.RemoteAddr()
		log.Debugf("raddr: %s", raddr.String())

		assert.Equal(t, "127.0.0.1", laddr.(*net.UDPAddr).IP.String(), "should match")
		assert.True(t, laddr.(*net.UDPAddr).Port != 0, "should match")
		assert.Equal(t, "127.0.0.1:1234", raddr.String(), "should match")
		assert.NoError(t, conn.Close(), "should succeed")
	})

	t.Run("Unexpected operations", func(t *testing.T) {
		// For portability of test, find a name of loopack interface name first
		var loName string
		ifs, err := net.Interfaces()
		assert.NoError(t, err, "should succeed")
		for _, ifc := range ifs {
			if ifc.Flags&net.FlagLoopback != 0 {
				loName = ifc.Name
				break
			}
		}

		nw := NewNet(nil)

		if len(loName) > 0 {
			// InterfaceByName
			ifc, err2 := nw.InterfaceByName(loName)
			assert.NoError(t, err2, "should succeed")
			assert.Equal(t, loName, ifc.Name, "should match")

			// getInterface
			_, err2 = nw.getInterface(loName)
			assert.Error(t, err2, "should fail")
		}

		_, err = nw.InterfaceByName("foo0")
		assert.Error(t, err, "should fail")

		// setRouter
		err = nw.setRouter(nil)
		assert.Error(t, err, "should fail")

		// onInboundChunk (shouldn't crash)
		nw.onInboundChunk(nil)

		// getStaticIPs
		ips := nw.getStaticIPs()
		assert.Nil(t, ips, "should be nil")
	})
}
