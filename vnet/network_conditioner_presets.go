package vnet

import "time"

// NetworkConditionerPreset represents a kind of NetworkConditioner with pre-defined values.
type NetworkConditionerPreset int

const (
	// NetworkConditionerPresetNone represents a NetworkConditioner with no effect.
	NetworkConditionerPresetNone NetworkConditionerPreset = iota + 1

	// NetworkConditionerPresetFullLoss represents a NetworkConditioner with 100% packet loss.
	NetworkConditionerPresetFullLoss

	// NetworkConditionerPreset3G represents a NetworkConditioner which replicates typical 3G network condition.
	NetworkConditionerPreset3G

	// NetworkConditionerPresetDSL represents a NetworkConditioner which replicates typical DSL network condition.
	NetworkConditionerPresetDSL

	// NetworkConditionerPresetEdge represents a NetworkConditioner which replicates typical EDGE network condition.
	NetworkConditionerPresetEdge

	// NetworkConditionerPresetLTE represents a NetworkConditioner which replicates typical LTE network condition.
	NetworkConditionerPresetLTE

	// NetworkConditionerPresetVeryBadNetwork represents a NetworkConditioner which replicates a very bad network condition.
	NetworkConditionerPresetVeryBadNetwork

	// NetworkConditionerPresetWiFi represents a NetworkConditioner which replicates typical WiFi network condition.
	NetworkConditionerPresetWiFi
)

// NewNetworkConditioner creates a new NetworkConditioner from the provided preset.
// If invalid preset, returns a NetworkConditioner with no effect (same as NetworkConditionerPresetNone).
func NewNetworkConditioner(preset NetworkConditionerPreset) *NetworkConditioner {
	switch preset {
	case NetworkConditionerPresetFullLoss:
		bandwidth := uint32(0)

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &bandwidth,
				PacketLoss:   NetworkConditionPacketLossDenominator,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &bandwidth,
				PacketLoss:   NetworkConditionPacketLossDenominator,
			},
		}

	case NetworkConditionerPreset3G:
		inBandwidth := uint32(780)
		outBandwidth := uint32(330)
		delay := 100 * time.Millisecond

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &inBandwidth,
				Latency:      delay,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &outBandwidth,
				Latency:      delay,
			},
		}

	case NetworkConditionerPresetDSL:
		inBandwidth := uint32(2000)
		outBandwidth := uint32(256)
		delay := 5 * time.Millisecond

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &inBandwidth,
				Latency:      delay,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &outBandwidth,
				Latency:      delay,
			},
		}

	case NetworkConditionerPresetEdge:
		inBandwidth := uint32(240)
		outBandwidth := uint32(200)
		inDelay := 400 * time.Millisecond
		outDelay := 440 * time.Millisecond

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &inBandwidth,
				Latency:      inDelay,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &outBandwidth,
				Latency:      outDelay,
			},
		}

	case NetworkConditionerPresetLTE:
		inBandwidth := uint32(50000)
		outBandwidth := uint32(10000)
		inDelay := 50 * time.Millisecond
		outDelay := 65 * time.Millisecond

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &inBandwidth,
				Latency:      inDelay,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &outBandwidth,
				Latency:      outDelay,
			},
		}

	case NetworkConditionerPresetVeryBadNetwork:
		bandwidth := uint32(1000)
		delay := 500 * time.Millisecond
		// 10% packet loss
		packetLoss := uint32(0.1 * float64(NetworkConditionPacketLossDenominator))

		settings := NetworkCondition{
			MaxBandwidth: &bandwidth,
			Latency:      delay,
			PacketLoss:   packetLoss,
		}

		return &NetworkConditioner{
			DownLink: settings,
			UpLink:   settings,
		}

	case NetworkConditionerPresetWiFi:
		inBandwidth := uint32(40000)
		outBandwidth := uint32(33000)
		delay := 1 * time.Millisecond

		return &NetworkConditioner{
			DownLink: NetworkCondition{
				MaxBandwidth: &inBandwidth,
				Latency:      delay,
			},
			UpLink: NetworkCondition{
				MaxBandwidth: &outBandwidth,
				Latency:      delay,
			},
		}

	case NetworkConditionerPresetNone:
		fallthrough
	default:
		return &NetworkConditioner{
			DownLink: NetworkCondition{},
			UpLink:   NetworkCondition{},
		}
	}
}
