package vnet

import (
	"math/rand"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	// NetworkConditionPacketLossDenominator is the denominator by which the `NetworkCondition.PacketLoss` value
	// will be divided.
	NetworkConditionPacketLossDenominator uint32 = 1000

	kiloBytesInBytes = 1000
	// Value chosen arbitrarily, there may be a better one.
	bandwidthComputeThreshold = 2 * time.Second
)

// NetworkConditioner hold the NetworkCondition of DownLink and UpLink channels.
type NetworkConditioner struct {
	// Internals stats

	// Total of downLink bytes
	// Must be accessed atomically.
	inBytes                  uint64
	pendingInBytes           uint64
	lastInByteChunkTimestamp int64

	// Total of upLink bytes
	// Must be accessed atomically.
	outBytes                  uint64
	pendingOutBytes           uint64
	lastOutByteChunkTimestamp int64

	// DownLink represents the DownLink condition.
	DownLink NetworkCondition
	// UpLink represents the UpLink condition.
	UpLink NetworkCondition
}

func (n *NetworkConditioner) handleDownLink(c Chunk) bool {
	atomic.AddUint64(&n.pendingInBytes, uint64(len(c.UserData())))

	bandwidthLimiterSleep := n.bandwidthLimiter(&n.DownLink.MaxBandwidth, &n.lastInByteChunkTimestamp, &n.inBytes, &n.pendingInBytes)

	// Apply constant latency if required
	if sleepNeeded := time.Duration(atomic.LoadInt64((*int64)(&n.DownLink.Latency))) - bandwidthLimiterSleep; sleepNeeded > 0 {
		time.Sleep(sleepNeeded)
	}

	return n.DownLink.shouldDropPacket()
}

func (n *NetworkConditioner) handleUpLink(c Chunk) bool {
	atomic.AddUint64(&n.pendingOutBytes, uint64(len(c.UserData())))

	bandwidthLimiterSleep := n.bandwidthLimiter(&n.UpLink.MaxBandwidth, &n.lastOutByteChunkTimestamp, &n.outBytes, &n.pendingOutBytes)

	// Apply constant latency if required
	if sleepNeeded := time.Duration(atomic.LoadInt64((*int64)(&n.UpLink.Latency))) - bandwidthLimiterSleep; sleepNeeded > 0 {
		time.Sleep(sleepNeeded)
	}

	return n.UpLink.shouldDropPacket()
}

func (n *NetworkConditioner) bandwidthLimiter(maxBandwidth **uint32, lastTimestamp *int64, totalBytes *uint64, pendingBytes *uint64) time.Duration {
	// Is safe because refers to a `MaxBandwidth` field in an initialized NetworkCondition struct.
	// nolint:gosec
	bandwidthLimit := (*uint32)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(maxBandwidth))))
	bandwidthLimiterSleep := 0 * time.Millisecond

	if bandwidthLimit != nil {
		now := time.Now().UnixNano()
		isFirstLoop := atomic.CompareAndSwapInt64(lastTimestamp, 0, now)

		if !isFirstLoop {
			duration := now - atomic.LoadInt64(lastTimestamp)
			if time.Duration(duration) > bandwidthComputeThreshold {
				previouslyPending := atomic.SwapUint64(pendingBytes, 0)

				bandwidthKbps := (float64(previouslyPending) / float64(duration) * float64(time.Second)) / kiloBytesInBytes

				atomic.AddUint64(totalBytes, previouslyPending)
				atomic.StoreInt64(lastTimestamp, now)

				if bandwidthKbps > float64(*bandwidthLimit) {
					exceeding := bandwidthKbps - float64(*bandwidthLimit)
					bandwidthLimiterSleep = time.Duration(exceeding * float64(time.Second))

					time.Sleep(bandwidthLimiterSleep)
				}
			}
		}
	}

	return bandwidthLimiterSleep
}

// NetworkCondition represents the network condition parameter of a data transmission channel direction.
type NetworkCondition struct {
	// Latency represents the network delay.
	Latency time.Duration
	// MaxBandwidth is the maximum bandwidth, in Kbps.
	MaxBandwidth *uint32
	// PacketLoss in percentage.
	// This value will be divided by NetworkConditionPacketLossDenominator.
	// Any quotient > 1.00 is meaningless.
	PacketLoss uint32
}

// Updates atomically each member of the current NetworkCondition with the new one.
func (c *NetworkCondition) update(newCondition NetworkCondition) {
	// Each member is atomically updated, but not the whole struct.
	// This isn't a problem, as it may only affect a few packets – and removes the need for locks.
	// nolint:gosec
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.MaxBandwidth)), unsafe.Pointer(newCondition.MaxBandwidth))
	atomic.StoreInt64((*int64)(&c.Latency), int64(newCondition.Latency))
	atomic.StoreUint32(&c.PacketLoss, newCondition.PacketLoss)
}

func (c *NetworkCondition) shouldDropPacket() bool {
	packetLoss := atomic.LoadUint32(&c.PacketLoss)

	// nolint:gosec
	if packetLoss > 0 && uint32(rand.Int31n(int32(NetworkConditionPacketLossDenominator))) < packetLoss {
		return true
	}

	return false
}
