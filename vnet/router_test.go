package vnet

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

type dummyNIC struct {
	Net
	onInboundChunkHandler func(Chunk)
}

// hijack onInboundChunk
func (v *dummyNIC) onInboundChunk(c Chunk) {
	v.onInboundChunkHandler(c)
}

func getIPAddr(n NIC) (string, error) {
	eth0, err := n.getInterface("eth0")
	if err != nil {
		return "", err
	}

	addrs, err := eth0.Addrs()
	if err != nil {
		return "", err
	}

	if len(addrs) != 1 {
		return "", fmt.Errorf("there must be one address")
	}

	return addrs[0].(*net.IPNet).IP.String(), nil
}

func TestRouterStandalone(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("CIDR parsing", func(t *testing.T) {
		r, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})

		assert.Nil(t, err, "should succeed")
		assert.Equal(t, "1.2.3.0", r.ipv4Net.IP.String(), "ip should match")
		assert.Equal(t, "ffffff00", r.ipv4Net.Mask.String(), "mask should match")
	})

	t.Run("assignIPAddress", func(t *testing.T) {
		r, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")

		for i := 1; i < 255; i++ {
			ip, err2 := r.assignIPAddress()
			assert.Nil(t, err2, "should succeed")
			assert.Equal(t, byte(1), ip[0], "should match")
			assert.Equal(t, byte(2), ip[1], "should match")
			assert.Equal(t, byte(3), ip[2], "should match")
			assert.Equal(t, byte(i), ip[3], "should match")
		}

		_, err = r.assignIPAddress()
		assert.NotNil(t, err, "should fail")
	})

	t.Run("AddNet", func(t *testing.T) {
		r, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")

		nic := NewNet(&NetConfig{})
		assert.NotNil(t, nic, "should succeed")

		err = r.AddNet(nic)
		assert.Nil(t, err, "should succeed")

		// Now, eth0 must have one address assigned
		eth0, err := nic.v.getInterface("eth0")
		assert.Nil(t, err, "should succeed")
		addrs, err := eth0.Addrs()
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, 1, len(addrs), "should match")
		assert.Equal(t, "ip+net", addrs[0].Network(), "should match")
		assert.Equal(t, "1.2.3.1/24", addrs[0].String(), "should match")
		assert.Equal(t, "1.2.3.1", addrs[0].(*net.IPNet).IP.String(), "should match")
	})

	t.Run("routing", func(t *testing.T) {
		var nCbs0 int32
		doneCh := make(chan struct{})
		r, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := 0; i < 2; i++ {
			anic := NewNet(&NetConfig{})
			assert.NotNil(t, anic, "should succeed")

			nic[i] = &dummyNIC{
				Net: *anic,
			}

			err2 := r.AddNet(nic[i])
			assert.Nil(t, err2, "should succeed")

			// Now, eth0 must have one address assigned
			eth0, err2 := nic[i].getInterface("eth0")
			assert.Nil(t, err2, "should succeed")
			addrs, err2 := eth0.Addrs()
			assert.Nil(t, err2, "should succeed")
			assert.Equal(t, 1, len(addrs), "should match")
			ip[i] = &net.UDPAddr{
				IP:   addrs[0].(*net.IPNet).IP,
				Port: 1111 * (i + 1),
			}
		}

		nic[0].onInboundChunkHandler = func(c Chunk) {
			log.Debugf("nic[0] received: %s", c.String())
			atomic.AddInt32(&nCbs0, 1)
		}

		nic[1].onInboundChunkHandler = func(c Chunk) {
			log.Debugf("nic[1] received: %s", c.String())
			close(doneCh)
		}

		err = r.Start()
		assert.Nil(t, err, "should succeed")

		c := newChunkUDP(ip[0], ip[1])
		r.push(c)

		<-doneCh
		err = r.Stop()
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, int32(0), atomic.LoadInt32(&nCbs0), "should be zero")
	})

	t.Run("AddChunkFilter", func(t *testing.T) {
		var nCbs0 int32
		var nCbs1 int32
		r, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := 0; i < 2; i++ {
			anic := NewNet(&NetConfig{})
			assert.NotNil(t, anic, "should succeed")

			nic[i] = &dummyNIC{
				Net: *anic,
			}

			err2 := r.AddNet(nic[i])
			assert.Nil(t, err2, "should succeed")

			// Now, eth0 must have one address assigned
			eth0, err2 := nic[i].getInterface("eth0")
			assert.Nil(t, err2, "should succeed")
			addrs, err2 := eth0.Addrs()
			assert.Nil(t, err2, "should succeed")
			assert.Equal(t, 1, len(addrs), "should match")
			ip[i] = &net.UDPAddr{
				IP:   addrs[0].(*net.IPNet).IP,
				Port: 1111 * (i + 1),
			}
		}

		nic[0].onInboundChunkHandler = func(c Chunk) {
			log.Debugf("nic[0] received: %s", c.String())
			atomic.AddInt32(&nCbs0, 1)
		}

		var seq byte
		nic[1].onInboundChunkHandler = func(c Chunk) {
			log.Debugf("nic[1] received: %s", c.String())
			seq = c.UserData()[0]
			atomic.AddInt32(&nCbs1, 1)
		}

		// this creates a filter that block the first chunk
		makeFilter := func(name string) func(c Chunk) bool {
			n := 0
			return func(c Chunk) bool {
				pass := (n > 0)
				if pass {
					log.Debugf("%s passed %s", name, c.String())
				} else {
					log.Debugf("%s blocked %s", name, c.String())
				}
				n++
				return pass
			}
		}

		// filter 1: block first one
		r.AddChunkFilter(makeFilter("filter1"))

		// filter 2: block first one
		r.AddChunkFilter(makeFilter("filter2"))

		err = r.Start()
		assert.Nil(t, err, "should succeed")

		// send 3 packets
		for i := 0; i < 3; i++ {
			c := newChunkUDP(ip[0], ip[1])
			c.userData = make([]byte, 1)
			c.userData[0] = byte(i) // 1-byte seq num
			r.push(c)
		}

		time.Sleep(50 * time.Millisecond)

		err = r.Stop()
		assert.Nil(t, err, "should succeed")
		assert.Equal(t, int32(0), atomic.LoadInt32(&nCbs0), "should be zero")
		assert.Equal(t, int32(1), atomic.LoadInt32(&nCbs1), "should be zero")
		assert.Equal(t, byte(2), seq, "should be the last chunk")
	})
}

func TestRouterDelay(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	subTest := func(t *testing.T, title string, minDelay, maxJitter time.Duration) {
		t.Run(title, func(t *testing.T) {
			const margin = 8 * time.Millisecond
			var nCBs int32
			doneCh := make(chan struct{})
			r, err := NewRouter(&RouterConfig{
				CIDR:          "1.2.3.0/24",
				MinDelay:      minDelay,
				MaxJitter:     maxJitter,
				LoggerFactory: loggerFactory,
			})
			assert.Nil(t, err, "should succeed")

			nic := make([]*dummyNIC, 2)
			ip := make([]*net.UDPAddr, 2)

			for i := 0; i < 2; i++ {
				anic := NewNet(&NetConfig{})
				assert.NotNil(t, anic, "should succeed")

				nic[i] = &dummyNIC{
					Net: *anic,
				}

				err2 := r.AddNet(nic[i])
				assert.Nil(t, err2, "should succeed")

				// Now, eth0 must have one address assigned
				eth0, err2 := nic[i].getInterface("eth0")
				assert.Nil(t, err2, "should succeed")
				addrs, err2 := eth0.Addrs()
				assert.Nil(t, err2, "should succeed")
				assert.Equal(t, 1, len(addrs), "should match")
				ip[i] = &net.UDPAddr{
					IP:   addrs[0].(*net.IPNet).IP,
					Port: 1111 * (i + 1),
				}
			}

			var delayRes []time.Duration
			nPkts := 1

			nic[0].onInboundChunkHandler = func(c Chunk) {}

			nic[1].onInboundChunkHandler = func(c Chunk) {
				delay := time.Since(c.getTimestamp())
				delayRes = append(delayRes, delay)
				n := atomic.AddInt32(&nCBs, 1)
				if n == int32(nPkts) {
					close(doneCh)
				}
			}

			err = r.Start()
			assert.Nil(t, err, "should succeed")

			for i := 0; i < nPkts; i++ {
				c := newChunkUDP(ip[0], ip[1])
				r.push(c)
				time.Sleep(50 * time.Millisecond)
			}

			<-doneCh
			err = r.Stop()
			assert.Nil(t, err, "should succeed")

			// Validate the amount of delays
			for _, d := range delayRes {
				log.Infof("min delay : %v", minDelay)
				log.Infof("max jitter: %v", maxJitter)
				log.Infof("actual delay: %v", d)
				assert.True(t, d >= minDelay, "should delay >= 20ms")
				assert.True(t, d <= (minDelay+maxJitter+margin), "should delay <= minDelay + maxJitter")
				// Note: actual delay should be within 30ms but giving a 8ms
				// margin for possible extra delay
				// (e.g. wakeup delay, debug logs, etc)
			}
		})
	}

	subTest(t, "Delay only", 20*time.Millisecond, 0)
	subTest(t, "Jitter only", 0, 10*time.Millisecond)
	subTest(t, "Delay and Jitter", 20*time.Millisecond, 10*time.Millisecond)
}

func TestRouterOneChild(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("lan to wan", func(t *testing.T) {
		doneCh := make(chan struct{})

		// WAN
		wan, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")
		assert.NotNil(t, wan, "should succeed")

		wanNet := &dummyNIC{
			Net: *NewNet(&NetConfig{}),
		}

		err = wan.AddNet(wanNet)
		assert.Nil(t, err, "should succeed")

		// Now, eth0 must have one address assigned
		wanIP, err := getIPAddr(wanNet)
		log.Debugf("wanIP: %s", wanIP)

		// LAN
		lan, err := NewRouter(&RouterConfig{
			CIDR:          "192.168.0.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.Nil(t, err, "should succeed")
		assert.NotNil(t, lan, "should succeed")

		lanNet := &dummyNIC{
			Net: *NewNet(&NetConfig{}),
		}
		err = lan.AddNet(lanNet)
		assert.Nil(t, err, "should succeed")

		// Now, eth0 must have one address assigned
		lanIP, err := getIPAddr(lanNet)
		log.Debugf("lanIP: %s", lanIP)

		err = wan.AddRouter(lan)
		assert.Nil(t, err, "should succeed")

		lanNet.onInboundChunkHandler = func(c Chunk) {
			log.Debugf("lanNet received: %s", c.String())
			close(doneCh)
		}

		wanNet.onInboundChunkHandler = func(c Chunk) {
			log.Debugf("wanNet received: %s", c.String())

			// echo the chunk
			echo := c.Clone().(*chunkUDP)
			err = echo.setSourceAddr(c.(*chunkUDP).DestinationAddr().String())
			assert.NoError(t, err, "should succeed")
			err = echo.setDestinationAddr(c.(*chunkUDP).SourceAddr().String())
			assert.NoError(t, err, "should succeed")

			log.Debug("wan.push being called..")
			wan.push(echo)
			log.Debug("wan.push called!")
		}

		err = wan.Start()
		assert.Nil(t, err, "should succeed")

		c := newChunkUDP(
			&net.UDPAddr{
				IP:   net.ParseIP(lanIP),
				Port: 1234,
			},
			&net.UDPAddr{
				IP:   net.ParseIP(wanIP),
				Port: 5678,
			},
		)

		log.Debugf("sending %s", c.String())

		lan.push(c)

		<-doneCh
		err = wan.Stop()
		assert.Nil(t, err, "should succeed")
	})

}
