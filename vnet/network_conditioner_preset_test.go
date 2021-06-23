package vnet

import (
	"testing"
)

func TestNetworkConditionerPresets(t *testing.T) {
	presets := [...]NetworkConditionerPreset{
		NetworkConditionerPresetNone,
		NetworkConditionerPresetFullLoss,
		NetworkConditionerPreset3G,
		NetworkConditionerPresetDSL,
		NetworkConditionerPresetEdge,
		NetworkConditionerPresetLTE,
		NetworkConditionerPresetVeryBadNetwork,
		NetworkConditionerPresetWiFi,
	}

	for _, preset := range presets {
		conditioner := NewNetworkConditioner(preset)

		if conditioner == nil {
			t.Fail()
		}
	}
}
