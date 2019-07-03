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

func TestNATTypeDefauts(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	nat := newNAT(&natConfig{
		natType:       NATType{},
		mappedIP:      "1.2.3.4",
		loggerFactory: loggerFactory,
	})

	assert.Equal(t, EndpointIndependent, nat.natType.MappingBehavior, "should match")
	assert.Equal(t, EndpointIndependent, nat.natType.FilteringBehavior, "should match")
	assert.False(t, nat.natType.Hairpining, "should be false")
	assert.False(t, nat.natType.PortPreservation, "should be false")
	assert.Equal(t, defaultNATMappingLifeTime, nat.natType.MappingLifeTime, "should be false")
}

func TestNATMappingBehavior(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("full-cone NAT", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointIndependent,
				FilteringBehavior: EndpointIndependent,
				Hairpining:        false,
				MappingLifeTime:   30 * time.Second,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP("5.6.7.8"),
			Port: 5678,
		}

		oic := newChunkUDP(src, dst)

		oec, err := nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-original  : %s\n", oic.String())
		log.Debugf("o-translated: %s\n", oec.String())

		iec := newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		log.Debugf("i-original  : %s\n", iec.String())

		iic, err := nat.translateInbound(iec)
		assert.Nil(t, err, "should succeed")

		log.Debugf("i-translated: %s\n", iic.String())

		assert.Equal(t,
			oic.SourceAddr().String(),
			iic.(*chunkUDP).DestinationAddr().String(),
			"should match")

		// packet with dest addr that does not existing in the mapping table
		// will be dropped
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort + 1,
			},
		)

		_, err = nat.translateInbound(iec)
		log.Debug(err.Error())
		assert.NotNil(t, err, "should fail (dropped)")

		// packet from any addr will be accepted (full-cone)
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: 7777,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		_, err = nat.translateInbound(iec)
		assert.Nil(t, err, "should succeed")
	})

	t.Run("addr-restricted-cone NAT", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointIndependent,
				FilteringBehavior: EndpointAddrDependent,
				Hairpining:        false,
				MappingLifeTime:   30 * time.Second,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP("5.6.7.8"),
			Port: 5678,
		}

		oic := newChunkUDP(src, dst)
		log.Debugf("o-original  : %s\n", oic.String())

		oec, err := nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-translated: %s\n", oec.String())

		iec := newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		log.Debugf("i-original  : %s\n", iec.String())

		iic, err := nat.translateInbound(iec)
		assert.Nil(t, err, "should succeed")

		log.Debugf("i-translated: %s\n", iic.String())

		assert.Equal(t,
			oic.SourceAddr().String(),
			iic.(*chunkUDP).DestinationAddr().String(),
			"should match")

		// packet with dest addr that does not existing in the mapping table
		// will be dropped
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort + 1,
			},
		)

		_, err = nat.translateInbound(iec)
		log.Debug(err.Error())
		assert.NotNil(t, err, "should fail (dropped)")

		// packet from any port will be accepted (restricted-cone)
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: 7777,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		_, err = nat.translateInbound(iec)
		assert.Nil(t, err, "should succeed")

		// packet from different addr will be droped (restricted-cone)
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("6.6.6.6"),
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		_, err = nat.translateInbound(iec)
		log.Debug(err.Error())
		assert.NotNil(t, err, "should fail (dropped)")
	})

	t.Run("port-restricted-cone NAT", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointIndependent,
				FilteringBehavior: EndpointAddrPortDependent,
				Hairpining:        false,
				MappingLifeTime:   30 * time.Second,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP("5.6.7.8"),
			Port: 5678,
		}

		oic := newChunkUDP(src, dst)
		log.Debugf("o-original  : %s\n", oic.String())

		oec, err := nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-translated: %s\n", oec.String())

		iec := newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		log.Debugf("i-original  : %s\n", iec.String())

		iic, err := nat.translateInbound(iec)
		assert.Nil(t, err, "should succeed")

		log.Debugf("i-translated: %s\n", iic.String())

		assert.Equal(t,
			oic.SourceAddr().String(),
			iic.(*chunkUDP).DestinationAddr().String(),
			"should match")

		// packet with dest addr that does not existing in the mapping table
		// will be dropped
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort + 1,
			},
		)

		_, err = nat.translateInbound(iec)
		assert.NotNil(t, err, "should fail (dropped)")

		// packet from different port will be dropped (port-restricted-cone)
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: 7777,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		_, err = nat.translateInbound(iec)
		assert.NotNil(t, err, "should fail (dropped)")

		// packet from different addr will be droped (restricted-cone)
		iec = newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("6.6.6.6"),
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		_, err = nat.translateInbound(iec)
		assert.NotNil(t, err, "should fail (dropped)")
	})

	t.Run("symmetric NAT addr dependent mapping", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointAddrDependent,
				FilteringBehavior: EndpointAddrDependent,
				Hairpining:        false,
				MappingLifeTime:   30 * time.Second,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		oic1 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.8"),
				Port: 5678,
			},
		)

		oic2 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.100"),
				Port: 5678,
			},
		)

		oic3 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.8"),
				Port: 6000,
			},
		)

		log.Debugf("o-original  : %s\n", oic1.String())
		log.Debugf("o-original  : %s\n", oic2.String())
		log.Debugf("o-original  : %s\n", oic3.String())

		oec1, err := nat.translateOutbound(oic1)
		assert.Nil(t, err, "should succeed")

		oec2, err := nat.translateOutbound(oic2)
		assert.Nil(t, err, "should succeed")

		oec3, err := nat.translateOutbound(oic3)
		assert.Nil(t, err, "should succeed")

		assert.Equal(t, 2, len(nat.outboundMap), "should match")
		assert.Equal(t, 2, len(nat.inboundMap), "should match")

		log.Debugf("o-translated: %s\n", oec1.String())
		log.Debugf("o-translated: %s\n", oec2.String())
		log.Debugf("o-translated: %s\n", oec3.String())

		assert.NotEqual(t, oec1.(*chunkUDP).sourcePort, oec2.(*chunkUDP).sourcePort, "should not match")
		assert.Equal(t, oec1.(*chunkUDP).sourcePort, oec3.(*chunkUDP).sourcePort, "should match")
	})

	t.Run("symmetric NAT port dependent mapping", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointAddrPortDependent,
				FilteringBehavior: EndpointAddrPortDependent,
				Hairpining:        false,
				MappingLifeTime:   30 * time.Second,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		oic1 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.8"),
				Port: 5678,
			},
		)

		oic2 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.100"),
				Port: 5678,
			},
		)

		oic3 := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP("192.168.0.2"),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP("5.6.7.8"),
				Port: 6000,
			},
		)

		log.Debugf("o-original  : %s\n", oic1.String())
		log.Debugf("o-original  : %s\n", oic2.String())
		log.Debugf("o-original  : %s\n", oic3.String())

		oec1, err := nat.translateOutbound(oic1)
		assert.Nil(t, err, "should succeed")

		oec2, err := nat.translateOutbound(oic2)
		assert.Nil(t, err, "should succeed")

		oec3, err := nat.translateOutbound(oic3)
		assert.Nil(t, err, "should succeed")

		assert.Equal(t, 3, len(nat.outboundMap), "should match")
		assert.Equal(t, 3, len(nat.inboundMap), "should match")

		log.Debugf("o-translated: %s\n", oec1.String())
		log.Debugf("o-translated: %s\n", oec2.String())
		log.Debugf("o-translated: %s\n", oec3.String())

		assert.NotEqual(t, oec1.(*chunkUDP).sourcePort, oec2.(*chunkUDP).sourcePort, "should not match")
		assert.NotEqual(t, oec1.(*chunkUDP).sourcePort, oec3.(*chunkUDP).sourcePort, "should match")
	})
}

func TestNATMappingTimeout(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("refresh on outbound", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointIndependent,
				FilteringBehavior: EndpointIndependent,
				Hairpining:        false,
				MappingLifeTime:   100 * time.Millisecond,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP("5.6.7.8"),
			Port: 5678,
		}

		oic := newChunkUDP(src, dst)

		oec, err := nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-original  : %s\n", oic.String())
		log.Debugf("o-translated: %s\n", oec.String())

		// record mapped addr
		mapped := oec.(*chunkUDP).SourceAddr().String()

		time.Sleep(75 * time.Millisecond)

		// refresh
		oec, err = nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-original  : %s\n", oic.String())
		log.Debugf("o-translated: %s\n", oec.String())

		assert.Equal(t, mapped, oec.(*chunkUDP).SourceAddr().String(), "mapped addr should match")

		// sleep long enough for the mapping to expire
		time.Sleep(125 * time.Millisecond)

		// refresh after expiration
		oec, err = nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-original  : %s\n", oic.String())
		log.Debugf("o-translated: %s\n", oec.String())

		assert.NotEqual(t, mapped, oec.(*chunkUDP).SourceAddr().String(), "mapped addr should not match")
	})

	t.Run("outbound detects timeout", func(t *testing.T) {
		nat := newNAT(&natConfig{
			natType: NATType{
				MappingBehavior:   EndpointIndependent,
				FilteringBehavior: EndpointIndependent,
				Hairpining:        false,
				MappingLifeTime:   100 * time.Millisecond,
			},
			mappedIP:      "1.2.3.4",
			loggerFactory: loggerFactory,
		})

		src := &net.UDPAddr{
			IP:   net.ParseIP("192.168.0.2"),
			Port: 1234,
		}
		dst := &net.UDPAddr{
			IP:   net.ParseIP("5.6.7.8"),
			Port: 5678,
		}

		oic := newChunkUDP(src, dst)

		oec, err := nat.translateOutbound(oic)
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(nat.outboundMap), "should match")
		assert.Equal(t, 1, len(nat.inboundMap), "should match")

		log.Debugf("o-original  : %s\n", oic.String())
		log.Debugf("o-translated: %s\n", oec.String())

		// sleep long enough for the mapping to expire
		time.Sleep(125 * time.Millisecond)

		iec := newChunkUDP(
			&net.UDPAddr{
				IP:   dst.IP,
				Port: dst.Port,
			},
			&net.UDPAddr{
				IP:   oec.(*chunkUDP).sourceIP,
				Port: oec.(*chunkUDP).sourcePort,
			},
		)

		log.Debugf("i-original  : %s\n", iec.String())

		_, err = nat.translateInbound(iec)
		assert.NotNil(t, err, "should drop")
		assert.Equal(t, 0, len(nat.outboundMap), "should have no binding")
		assert.Equal(t, 0, len(nat.inboundMap), "should have no binding")
	})
}
