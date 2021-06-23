// +build !js

package vnet

import (
	"net"
	"testing"
	"time"
)

func TestNetworkConditionPacketLoss(t *testing.T) {
	src := &net.UDPAddr{
		IP:   net.ParseIP("192.168.0.2"),
		Port: 1234,
	}
	dst := &net.UDPAddr{
		IP:   net.ParseIP("192.168.0.3"),
		Port: 5678,
	}

	t.Run("FullLoss", func(t *testing.T) {
		conditioner := NewNetworkConditioner(NetworkConditionerPresetFullLoss)

		// Every packet should be dismissed
		for ndx := 0; ndx < 20; ndx++ {
			if !conditioner.handleDownLink(newChunkUDP(src, dst)) {
				t.FailNow()
			}

			if !conditioner.handleUpLink(newChunkUDP(src, dst)) {
				t.FailNow()
			}
		}
	})

	t.Run("HeavyLoss", func(t *testing.T) {
		// 80% packet loss
		packetLoss := uint32(0.8 * float64(NetworkConditionPacketLossDenominator))

		conditioner := &NetworkConditioner{
			DownLink: NetworkCondition{
				PacketLoss: packetLoss,
			},
			UpLink: NetworkCondition{
				PacketLoss: packetLoss,
			},
		}

		const totalPackets = 100

		downPackets := 0
		upPackets := 0

		for ndx := 0; ndx < totalPackets; ndx++ {
			if !conditioner.handleDownLink(newChunkUDP(src, dst)) {
				downPackets++
			}

			if !conditioner.handleUpLink(newChunkUDP(src, dst)) {
				upPackets++
			}
		}

		// Don't assert for specific values, just make sure there is loss.
		if upPackets == 0 || downPackets == 0 || upPackets == totalPackets || downPackets == totalPackets {
			t.FailNow()
		}
	})

	t.Run("NoLoss", func(t *testing.T) {
		conditioner := NewNetworkConditioner(NetworkConditionerPresetNone)

		const totalPackets = 100

		downPackets := 0
		upPackets := 0

		for ndx := 0; ndx < totalPackets; ndx++ {
			if !conditioner.handleDownLink(newChunkUDP(src, dst)) {
				downPackets++
			}

			if !conditioner.handleUpLink(newChunkUDP(src, dst)) {
				upPackets++
			}
		}

		// Don't assert for specific values, just make sure there is loss.
		if upPackets != totalPackets || downPackets != totalPackets {
			t.FailNow()
		}
	})
}

func TestNetworkConditionBandwidth(t *testing.T) {
	src := &net.UDPAddr{
		IP:   net.ParseIP("192.168.0.2"),
		Port: 1234,
	}
	dst := &net.UDPAddr{
		IP:   net.ParseIP("192.168.0.3"),
		Port: 5678,
	}

	downBandwidthCap := uint32(10)
	upBandwidthCap := uint32(20)

	conditioner := &NetworkConditioner{
		DownLink: NetworkCondition{
			MaxBandwidth: &downBandwidthCap,
		},
		UpLink: NetworkCondition{
			MaxBandwidth: &upBandwidthCap,
		},
	}

	testDuration := 10 * time.Second

	pck := newChunkUDP(src, dst)

	downKilobytes := make(chan int, 1)
	upKilobytes := make(chan int, 1)

	// DownLink loop
	go func() {
		timeout := time.NewTimer(testDuration)

		emittedPackets := 0

	loop:
		for {
			select {
			case <-timeout.C:
				break loop
			default:
				if !conditioner.handleDownLink(pck) {
					emittedPackets++
				}
			}
		}

		downKilobytes <- emittedPackets * len(pck.UserData()) / kiloBytesInBytes
	}()

	// UpLink loop
	go func() {
		timeout := time.NewTimer(testDuration)

		emittedPackets := 0

	loop:
		for {
			select {
			case <-timeout.C:
				break loop
			default:
				if !conditioner.handleUpLink(pck) {
					emittedPackets++
				}
			}
		}

		upKilobytes <- emittedPackets * len(pck.UserData()) / kiloBytesInBytes
	}()

	downBandwidth := float64(<-downKilobytes) / testDuration.Seconds()
	upBandwidth := float64(<-upKilobytes) / testDuration.Seconds()

	if downBandwidth > float64(downBandwidthCap) {
		t.FailNow()
	}

	if upBandwidth > float64(upBandwidthCap) {
		t.FailNow()
	}

	if upBandwidth < downBandwidth {
		t.FailNow()
	}
}
