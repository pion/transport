package vnet

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestRouterBandwidth(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	leftAddr, rightAddr := "1.2.3.4", "1.2.3.5"

	subTest := func(t *testing.T, capacity int, duration time.Duration) {
		wan, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)
		assert.NotNil(t, wan)

		leftNet := NewNet(&NetConfig{
			StaticIPs: []string{leftAddr},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Configure network bandwidth capacity:
		tbf, err := NewTokenBucketFilter(ctx, leftNet, TBFRate(capacity))
		assert.NoError(t, err)

		err = wan.AddNet(tbf)
		assert.NoError(t, err)

		rightNet := NewNet(&NetConfig{
			StaticIPs: []string{rightAddr},
		})
		err = wan.AddNet(rightNet)
		assert.NoError(t, err)

		err = wan.Start()
		assert.NoError(t, err)
		defer func() {
			err = wan.Stop()
			assert.NoError(t, err)
		}()

		done := make(chan struct{})
		type metrics struct {
			packets int
			bytes   int
		}
		received := make(chan metrics, 1000)
		sent := make(chan metrics, 1000)

		var logWG sync.WaitGroup
		logWG.Add(1)

		go func() {
			defer logWG.Done()
			bytesReceived := 0
			pktReceived := 0
			bytesSent := 0
			pktSent := 0

			totalPktSent := 0
			totalPktReceived := 0
			totalBytesReceived := 0
			var start time.Time

			defer func() {
				d := time.Since(start)
				lossRate := 100 * (1 - float64(totalPktReceived)/float64(totalPktSent))
				bits := float64(totalBytesReceived) * 8.0
				rate := bits / d.Seconds()

				assert.Less(t, rate, float64(capacity))

				mBitPerSecond := rate / float64(MBit)
				log.Infof("total packets received: %v / %v, lossrate=%.2f%%, throughput=%.2f Mb/s\n", totalPktReceived, totalPktSent, lossRate, mBitPerSecond)
			}()

			ticker := time.NewTicker(1 * time.Second)
			start = time.Now()
			lastLog := start
			for {
				select {
				case <-done:
					for r := range received {
						pktReceived += r.packets
						bytesReceived += r.bytes
						totalPktReceived += r.packets
						totalBytesReceived += r.bytes
					}
					for s := range sent {
						pktSent += s.packets
						bytesSent += s.bytes
						totalPktSent += s.packets
					}
					return
				case r := <-received:
					pktReceived += r.packets
					bytesReceived += r.bytes
					totalPktReceived += r.packets
					totalBytesReceived += r.bytes
				case s := <-sent:
					pktSent += s.packets
					bytesSent += s.bytes
					totalPktSent += s.packets

				case now := <-ticker.C:
					d := now.Sub(lastLog)
					lastLog = now
					bits := float64(bytesReceived) * 8
					rate := bits / d.Seconds()
					rateInMbit := rate / float64(MBit)
					log.Infof("sent: %v B / %v P, received %v B / %v P => %.2f Mb/s\n", bytesSent, pktSent, bytesReceived, pktReceived, rateInMbit)
					pktReceived = 0
					bytesReceived = 0
					pktSent = 0
					bytesSent = 0
				}
			}
		}()

		connLeft, err := leftNet.ListenPacket("udp4", fmt.Sprintf("%v:0", leftAddr))
		assert.NoError(t, err)

		go func() {
			defer close(received)
			for {
				buf := make([]byte, 1500)
				n, _, err1 := connLeft.ReadFrom(buf)
				if err1 != nil {
					break
				}
				received <- metrics{
					packets: 1,
					bytes:   n,
				}
			}
		}()

		connRight, err := rightNet.ListenPacket("udp", fmt.Sprintf("%v:0", rightAddr))
		assert.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		raddr := connLeft.LocalAddr()
		go func() {
			defer wg.Done()
			defer func() {
				err1 := connRight.Close()
				assert.NoError(t, err1)
			}()
			defer close(done)
			defer close(sent)
			timer := time.NewTicker(duration)
			for {
				select {
				case <-timer.C:
					return
				default:
				}
				buf := make([]byte, 1500)
				n, err1 := connRight.WriteTo(buf, raddr)
				assert.NoError(t, err1)
				sent <- metrics{
					packets: 1,
					bytes:   n,
				}
				time.Sleep(5 * time.Nanosecond)
			}
		}()

		wg.Wait()
		err = connLeft.Close()
		assert.NoError(t, err)
		logWG.Wait()
	}

	t.Run("Router bandwidth 500Kbit", func(t *testing.T) {
		subTest(t, 500*KBit, 5*time.Second)
	})

	time.Sleep(2 * time.Second)
	t.Run("Router bandwidth 1Mbit", func(t *testing.T) {
		subTest(t, 1*MBit, 5*time.Second)
	})

	time.Sleep(2 * time.Second)
	t.Run("Router bandwidth 2Mbit", func(t *testing.T) {
		subTest(t, 2*MBit, 5*time.Second)
	})
}
