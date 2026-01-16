// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

// oic: outbound internal chunk
// oec: outbound external chunk
// iic: inbound internal chunk
// iec: inbound external chunk

const demoIP = "1.2.3.4"

type natTestProto struct {
	name     string
	newAddr  func(ip net.IP, port int) net.Addr
	newChunk func(t *testing.T, src, dst net.Addr) Chunk
}

func natTestProtos() []natTestProto {
	return []natTestProto{
		{
			name: "udp",
			newAddr: func(ip net.IP, port int) net.Addr {
				return &net.UDPAddr{IP: ip, Port: port}
			},
			newChunk: func(t *testing.T, src, dst net.Addr) Chunk {
				t.Helper()
				srcAddr, ok := src.(*net.UDPAddr)
				if !ok {
					assert.FailNow(t, "expected *net.UDPAddr src, got %T", src)
				}
				dstAddr, ok := dst.(*net.UDPAddr)
				if !ok {
					assert.FailNow(t, "expected *net.UDPAddr dst, got %T", dst)
				}

				return newChunkUDP(srcAddr, dstAddr)
			},
		},
		{
			name: "tcp",
			newAddr: func(ip net.IP, port int) net.Addr {
				return &net.TCPAddr{IP: ip, Port: port}
			},
			newChunk: func(t *testing.T, src, dst net.Addr) Chunk {
				t.Helper()
				srcAddr, ok := src.(*net.TCPAddr)
				if !ok {
					assert.FailNow(t, "expected *net.TCPAddr src, got %T", src)
				}
				dstAddr, ok := dst.(*net.TCPAddr)
				if !ok {
					assert.FailNow(t, "expected *net.TCPAddr dst, got %T", dst)
				}

				return newChunkTCP(srcAddr, dstAddr, tcpACK)
			},
		},
	}
}

func natAddrIPPort(t *testing.T, addr net.Addr) (net.IP, int) {
	t.Helper()

	switch a := addr.(type) {
	case *net.UDPAddr:
		return a.IP, a.Port
	case *net.TCPAddr:
		return a.IP, a.Port
	default:
		assert.FailNow(t, "unexpected addr type %T", addr)

		return nil, 0
	}
}

func TestNATTypeDefaults(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	nat, err := newNAT(&natConfig{
		natType:       NATType{},
		mappedIPs:     []net.IP{net.ParseIP(demoIP)},
		loggerFactory: loggerFactory,
	})
	assert.NoError(t, err, "should succeed")

	assert.Equal(t, EndpointIndependent, nat.natType.MappingBehavior, "should match")
	assert.Equal(t, EndpointIndependent, nat.natType.FilteringBehavior, "should match")
	assert.False(t, nat.natType.Hairpinning, "should be false")
	assert.False(t, nat.natType.PortPreservation, "should be false")
	assert.Equal(t, defaultNATMappingLifeTime, nat.natType.MappingLifeTime, "should be false")
}

func TestNATMappingBehavior(t *testing.T) { //nolint:maintidx
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	for _, proto := range natTestProtos() {
		proto := proto
		t.Run(proto.name, func(t *testing.T) {
			t.Run("full-cone NAT", func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointIndependent,
						FilteringBehavior: EndpointIndependent,
						Hairpinning:       false,
						MappingLifeTime:   30 * time.Second,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")

				log.Debugf("o-original  : %s", oic.String())
				log.Debugf("o-translated: %s", oec.String())

				oecIP, oecPort := natAddrIPPort(t, oec.SourceAddr())
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort))

				log.Debugf("i-original  : %s", iec.String())

				iic, err := nat.translateInbound(iec)
				assert.Nil(t, err, "should succeed")

				log.Debugf("i-translated: %s", iic.String())

				assert.Equal(t, oic.SourceAddr().String(), iic.DestinationAddr().String(), "should match")

				// packet with dest addr that does not exist in the mapping table
				// will be dropped
				iec = proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort+1))

				_, err = nat.translateInbound(iec)
				log.Debug(err.Error())
				assert.NotNil(t, err, "should fail (dropped)")

				// packet from any addr will be accepted (full-cone)
				iec = proto.newChunk(t, proto.newAddr(dstIP, 7777), proto.newAddr(oecIP, oecPort))

				_, err = nat.translateInbound(iec)
				assert.Nil(t, err, "should succeed")
			})

			t.Run("addr-restricted-cone NAT", func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointIndependent,
						FilteringBehavior: EndpointAddrDependent,
						Hairpinning:       false,
						MappingLifeTime:   30 * time.Second,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))
				log.Debugf("o-original  : %s", oic.String())

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")
				log.Debugf("o-translated: %s", oec.String())

				// sending different (IP: 5.6.7.9) won't create a new mapping
				oic2 := proto.newChunk(t,
					proto.newAddr(srcIP, srcPort),
					proto.newAddr(net.ParseIP("5.6.7.9"), 9000),
				)
				oec2, err := nat.translateOutbound(oic2)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")
				log.Debugf("o-translated: %s", oec2.String())

				oecIP, oecPort := natAddrIPPort(t, oec.SourceAddr())
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort))

				log.Debugf("i-original  : %s", iec.String())

				iic, err := nat.translateInbound(iec)
				if !assert.NoError(t, err, "should succeed") {
					return
				}

				log.Debugf("i-translated: %s", iic.String())

				assert.Equal(t, oic.SourceAddr().String(), iic.DestinationAddr().String(), "should match")

				// packet with dest addr that does not exist in the mapping table
				// will be dropped
				iec = proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort+1))

				_, err = nat.translateInbound(iec)
				log.Debug(err.Error())
				assert.NotNil(t, err, "should fail (dropped)")

				// packet from any port will be accepted (restricted-cone)
				iec = proto.newChunk(t, proto.newAddr(dstIP, 7777), proto.newAddr(oecIP, oecPort))

				_, err = nat.translateInbound(iec)
				assert.Nil(t, err, "should succeed")

				// packet from different addr will be dropped (restricted-cone)
				iec = proto.newChunk(t, proto.newAddr(net.ParseIP("6.6.6.6"), dstPort), proto.newAddr(oecIP, oecPort))

				_, err = nat.translateInbound(iec)
				log.Debug(err.Error())
				assert.NotNil(t, err, "should fail (dropped)")
			})

			t.Run("port-restricted-cone NAT", func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointIndependent,
						FilteringBehavior: EndpointAddrPortDependent,
						Hairpinning:       false,
						MappingLifeTime:   30 * time.Second,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))
				log.Debugf("o-original  : %s", oic.String())

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")

				log.Debugf("o-translated: %s", oec.String())

				// sending different (IP: 5.6.7.9) won't create a new mapping
				oic2 := proto.newChunk(t,
					proto.newAddr(srcIP, srcPort),
					proto.newAddr(net.ParseIP("5.6.7.9"), 9000),
				)
				oec2, err := nat.translateOutbound(oic2)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")
				log.Debugf("o-translated: %s", oec2.String())

				oecIP, oecPort := natAddrIPPort(t, oec.SourceAddr())
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort))

				log.Debugf("i-original  : %s", iec.String())

				iic, err := nat.translateInbound(iec)
				assert.Nil(t, err, "should succeed")

				log.Debugf("i-translated: %s", iic.String())

				assert.Equal(t, oic.SourceAddr().String(), iic.DestinationAddr().String(), "should match")

				// packet with dest addr that does not exist in the mapping table
				// will be dropped
				iec = proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort+1))

				_, err = nat.translateInbound(iec)
				assert.NotNil(t, err, "should fail (dropped)")

				// packet from different port will be dropped (port-restricted-cone)
				iec = proto.newChunk(t, proto.newAddr(dstIP, 7777), proto.newAddr(oecIP, oecPort))

				_, err = nat.translateInbound(iec)
				assert.NotNil(t, err, "should fail (dropped)")

				// packet from different addr will be dropped (restricted-cone)
				iec = proto.newChunk(t, proto.newAddr(net.ParseIP("6.6.6.6"), dstPort), proto.newAddr(oecIP, oecPort))

				_, err = nat.translateInbound(iec)
				assert.NotNil(t, err, "should fail (dropped)")
			})

			t.Run("symmetric NAT addr dependent mapping", func(t *testing.T) { //nolint:dupl
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointAddrDependent,
						FilteringBehavior: EndpointAddrDependent,
						Hairpinning:       false,
						MappingLifeTime:   30 * time.Second,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				oic1 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.8"), 5678))
				oic2 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.100"), 5678))
				oic3 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.8"), 6000))

				log.Debugf("o-original  : %s", oic1.String())
				log.Debugf("o-original  : %s", oic2.String())
				log.Debugf("o-original  : %s", oic3.String())

				oec1, err := nat.translateOutbound(oic1)
				assert.Nil(t, err, "should succeed")

				oec2, err := nat.translateOutbound(oic2)
				assert.Nil(t, err, "should succeed")

				oec3, err := nat.translateOutbound(oic3)
				assert.Nil(t, err, "should succeed")

				assert.Equal(t, 2, len(nat.outboundMap), "should match")
				assert.Equal(t, 2, len(nat.inboundMap), "should match")

				log.Debugf("o-translated: %s", oec1.String())
				log.Debugf("o-translated: %s", oec2.String())
				log.Debugf("o-translated: %s", oec3.String())

				_, p1 := natAddrIPPort(t, oec1.SourceAddr())
				_, p2 := natAddrIPPort(t, oec2.SourceAddr())
				_, p3 := natAddrIPPort(t, oec3.SourceAddr())
				assert.NotEqual(t, p1, p2, "should not match")
				assert.Equal(t, p1, p3, "should match")
			})

			t.Run("symmetric NAT port dependent mapping", func(t *testing.T) { //nolint:dupl
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointAddrPortDependent,
						FilteringBehavior: EndpointAddrPortDependent,
						Hairpinning:       false,
						MappingLifeTime:   30 * time.Second,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				oic1 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.8"), 5678))
				oic2 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.100"), 5678))
				oic3 := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(net.ParseIP("5.6.7.8"), 6000))

				log.Debugf("o-original  : %s", oic1.String())
				log.Debugf("o-original  : %s", oic2.String())
				log.Debugf("o-original  : %s", oic3.String())

				oec1, err := nat.translateOutbound(oic1)
				assert.Nil(t, err, "should succeed")

				oec2, err := nat.translateOutbound(oic2)
				assert.Nil(t, err, "should succeed")

				oec3, err := nat.translateOutbound(oic3)
				assert.Nil(t, err, "should succeed")

				assert.Equal(t, 3, len(nat.outboundMap), "should match")
				assert.Equal(t, 3, len(nat.inboundMap), "should match")

				log.Debugf("o-translated: %s", oec1.String())
				log.Debugf("o-translated: %s", oec2.String())
				log.Debugf("o-translated: %s", oec3.String())

				_, p1 := natAddrIPPort(t, oec1.SourceAddr())
				_, p2 := natAddrIPPort(t, oec2.SourceAddr())
				_, p3 := natAddrIPPort(t, oec3.SourceAddr())
				assert.NotEqual(t, p1, p2, "should not match")
				assert.NotEqual(t, p1, p3, "should match")
			})
		})
	}
}

func TestNATMappingTimeout(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("refresh on outbound", func(t *testing.T) {
		for _, proto := range natTestProtos() {
			proto := proto
			t.Run(proto.name, func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointIndependent,
						FilteringBehavior: EndpointIndependent,
						Hairpinning:       false,
						MappingLifeTime:   100 * time.Millisecond,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")

				log.Debugf("o-original  : %s", oic.String())
				log.Debugf("o-translated: %s", oec.String())

				mapped := oec.SourceAddr().String()

				time.Sleep(75 * time.Millisecond)

				// refresh
				oec, err = nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")

				log.Debugf("o-original  : %s", oic.String())
				log.Debugf("o-translated: %s", oec.String())

				assert.Equal(t, mapped, oec.SourceAddr().String(), "mapped addr should match")

				// sleep long enough for the mapping to expire
				time.Sleep(125 * time.Millisecond)

				// refresh after expiration
				oec, err = nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")
				assert.NotEqual(t, mapped, oec.SourceAddr().String(), "mapped addr should not match")
			})
		}
	})

	t.Run("outbound detects timeout", func(t *testing.T) {
		for _, proto := range natTestProtos() {
			proto := proto
			t.Run(proto.name, func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						MappingBehavior:   EndpointIndependent,
						FilteringBehavior: EndpointIndependent,
						Hairpinning:       false,
						MappingLifeTime:   100 * time.Millisecond,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					loggerFactory: loggerFactory,
				})
				assert.NoError(t, err, "should succeed")

				srcIP := net.ParseIP("192.168.0.2")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Equal(t, 1, len(nat.outboundMap), "should match")
				assert.Equal(t, 1, len(nat.inboundMap), "should match")

				log.Debugf("o-original  : %s", oic.String())
				log.Debugf("o-translated: %s", oec.String())

				// sleep long enough for the mapping to expire
				time.Sleep(125 * time.Millisecond)

				oecIP, oecPort := natAddrIPPort(t, oec.SourceAddr())
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort))
				log.Debugf("i-original  : %s", iec.String())

				iic, err := nat.translateInbound(iec)
				assert.NotNil(t, err, "should drop")
				assert.Nil(t, iic, "should be nil")
				assert.Empty(t, nat.outboundMap, "should have no binding")
				assert.Empty(t, nat.inboundMap, "should have no binding")
			})
		}
	})
}

func TestNAT1To1Behavior(t *testing.T) { // nolint:cyclop
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("1:1 NAT with one mapping", func(t *testing.T) {
		for _, proto := range natTestProtos() {
			proto := proto
			t.Run(proto.name, func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						Mode: NATModeNAT1To1,
					},
					mappedIPs:     []net.IP{net.ParseIP(demoIP)},
					localIPs:      []net.IP{net.ParseIP("10.0.0.1")},
					loggerFactory: loggerFactory,
				})
				if !assert.NoError(t, err, "should succeed") {
					return
				}

				srcIP := net.ParseIP("10.0.0.1")
				srcPort := 1234
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(srcIP, srcPort), proto.newAddr(dstIP, dstPort))

				oec, err := nat.translateOutbound(oic)
				assert.Nil(t, err, "should succeed")
				assert.Empty(t, nat.outboundMap, "should match")
				assert.Empty(t, nat.inboundMap, "should match")

				log.Debugf("o-original  : %s", oic.String())
				log.Debugf("o-translated: %s", oec.String())
				assert.Equal(t, "1.2.3.4:1234", oec.SourceAddr().String(), "should match")

				oecIP, oecPort := natAddrIPPort(t, oec.SourceAddr())
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(oecIP, oecPort))
				log.Debugf("i-original  : %s", iec.String())

				iic, err := nat.translateInbound(iec)
				assert.Nil(t, err, "should succeed")
				log.Debugf("i-translated: %s", iic.String())
				assert.Equal(t, oic.SourceAddr().String(), iic.DestinationAddr().String(), "should match")
			})
		}
	})

	t.Run("1:1 NAT with more than one mapping", func(t *testing.T) {
		for _, proto := range natTestProtos() {
			proto := proto
			t.Run(proto.name, func(t *testing.T) {
				nat, err := newNAT(&natConfig{
					natType: NATType{
						Mode: NATModeNAT1To1,
					},
					mappedIPs: []net.IP{
						net.ParseIP(demoIP),
						net.ParseIP("1.2.3.5"),
					},
					localIPs: []net.IP{
						net.ParseIP("10.0.0.1"),
						net.ParseIP("10.0.0.2"),
					},
					loggerFactory: loggerFactory,
				})
				if !assert.NoError(t, err, "should succeed") {
					return
				}

				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678

				// outbound translation
				oic := proto.newChunk(t, proto.newAddr(net.ParseIP("10.0.0.1"), 1234), proto.newAddr(dstIP, dstPort))
				oec, err := nat.translateOutbound(oic)
				if !assert.NoError(t, err, "should succeed") {
					return
				}
				assert.Equal(t, "1.2.3.4:1234", oec.SourceAddr().String(), "should match")

				oic = proto.newChunk(t, proto.newAddr(net.ParseIP("10.0.0.2"), 1234), proto.newAddr(dstIP, dstPort))
				oec, err = nat.translateOutbound(oic)
				if !assert.NoError(t, err, "should succeed") {
					return
				}
				assert.Equal(t, "1.2.3.5:1234", oec.SourceAddr().String(), "should match")

				// inbound translation
				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(net.ParseIP(demoIP), 2525))
				iic, err := nat.translateInbound(iec)
				if !assert.NoError(t, err, "should succeed") {
					return
				}
				assert.Equal(t, "10.0.0.1:2525", iic.DestinationAddr().String(), "should match")

				iec = proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(net.ParseIP("1.2.3.5"), 9847))
				iic, err = nat.translateInbound(iec)
				if !assert.NoError(t, err, "should succeed") {
					return
				}
				assert.Equal(t, "10.0.0.2:9847", iic.DestinationAddr().String(), "should match")
			})
		}
	})

	t.Run("1:1 NAT failure", func(t *testing.T) {
		// 1:1 NAT requires more than one mapping
		_, err := newNAT(&natConfig{
			natType: NATType{
				Mode: NATModeNAT1To1,
			},
			loggerFactory: loggerFactory,
		})
		assert.Error(t, err, "should fail")

		// 1:1 NAT requires the same number of mappedIPs and localIPs
		_, err = newNAT(&natConfig{
			natType: NATType{
				Mode: NATModeNAT1To1,
			},
			mappedIPs: []net.IP{
				net.ParseIP(demoIP),
				net.ParseIP("1.2.3.5"),
			},
			localIPs: []net.IP{
				net.ParseIP("10.0.0.1"),
			},
			loggerFactory: loggerFactory,
		})
		assert.Error(t, err, "should fail")

		// drop outbound or inbound chunk with no route in 1:1 NAT

		nat, err := newNAT(&natConfig{
			natType: NATType{
				Mode: NATModeNAT1To1,
			},
			mappedIPs: []net.IP{
				net.ParseIP(demoIP),
			},
			localIPs: []net.IP{
				net.ParseIP("10.0.0.1"),
			},
			loggerFactory: loggerFactory,
		})
		assert.NoError(t, err, "should succeed")

		for _, proto := range natTestProtos() {
			proto := proto
			t.Run(proto.name, func(t *testing.T) {
				dstIP := net.ParseIP("5.6.7.8")
				dstPort := 5678
				oic := proto.newChunk(t, proto.newAddr(net.ParseIP("10.0.0.2"), 1234), proto.newAddr(dstIP, dstPort))
				oec, err := nat.translateOutbound(oic)
				if !assert.NoError(t, err, "should succeed") {
					return
				}
				assert.Nil(t, oec, "should be nil")

				iec := proto.newChunk(t, proto.newAddr(dstIP, dstPort), proto.newAddr(net.ParseIP("10.0.0.2"), 1234))
				_, err = nat.translateInbound(iec)
				assert.Error(t, err, "should fail")
			})
		}
	})
}
