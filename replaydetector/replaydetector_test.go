// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package replaydetector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	windowSize uint
	maxSeq     uint64
	input      []uint64
	valid      []bool
	latest     []bool
	expected   []uint64
}

const (
	largeSeq = 0x100000000000
	hugeSeq  = 0x1000000000000
)

var (
	_ CheckAccepter = (*slidingWindowDetector)(nil)
	_ CheckAccepter = (*wrappedSlidingWindowDetector)(nil)
)

var commonCases = map[string]testCase{ //nolint:gochecknoglobals
	"Continuous": {
		16, 0x0000FFFFFFFFFFFF,
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, true, true, true, true, true, true,
			true,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, true, true, true, true, true, true,
			true,
		},
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	},
	"ValidLargeJump": {
		16, 0x0000FFFFFFFFFFFF,
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, largeSeq, 11, largeSeq + 1, largeSeq + 2, largeSeq + 3},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, false, true, true, true,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, false, true, true, true,
		},
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, largeSeq, largeSeq + 1, largeSeq + 2, largeSeq + 3},
	},
	"InvalidLargeJump": {
		16, 0x0000FFFFFFFFFFFF,
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, hugeSeq, 11, 12, 13, 14, 15},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			false, true, true, true, true, true,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			false, true, true, true, true, true,
		},
		[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15},
	},
	"DuplicateAfterValidJump": {
		196, 0x0000FFFFFFFFFFFF,
		[]uint64{0, 1, 2, 129, 0, 1, 2},
		[]bool{
			true, true, true, true, false, false, false,
		},
		[]bool{
			true, true, true, true, false, false, false,
		},
		[]uint64{0, 1, 2, 129},
	},
	"DuplicateAfterInvalidJump": {
		196, 0x0000FFFFFFFFFFFF,
		[]uint64{0, 1, 2, hugeSeq, 0, 1, 2},
		[]bool{
			true, true, true, false, false, false, false,
		},
		[]bool{
			true, true, true, false, false, false, false,
		},
		[]uint64{0, 1, 2},
	},
	"ContinuousOffset": {
		16, 0x0000FFFFFFFFFFFF,
		[]uint64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, true,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, true,
		},
		[]uint64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114},
	},
	"Reordered": {
		128, 0x0000FFFFFFFFFFFF,
		[]uint64{96, 64, 16, 80, 32, 48, 8, 24, 88, 40, 128, 56, 72, 112, 104, 120},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, true, true,
		},
		[]bool{
			true, false, false, false, false, false, false, false, false, false,
			true, false, false, false, false, false,
		},
		[]uint64{96, 64, 16, 80, 32, 48, 8, 24, 88, 40, 128, 56, 72, 112, 104, 120},
	},
	"Old": {
		100, 0x0000FFFFFFFFFFFF,
		[]uint64{24, 32, 40, 48, 56, 64, 72, 80, 88, 96, 104, 112, 120, 128, 8, 16},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, false, false,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			true, true, true, true, false, false,
		},
		[]uint64{24, 32, 40, 48, 56, 64, 72, 80, 88, 96, 104, 112, 120, 128},
	},
	"ContinuousReplayed": {
		8, 0x0000FFFFFFFFFFFF,
		[]uint64{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			false, false, false, false, false, false, false, false, false, false,
		},
		[]bool{
			true, true, true, true, true, true, true, true, true, true,
			false, false, false, false, false, false, false, false, false, false,
		},
		[]uint64{16, 17, 18, 19, 20, 21, 22, 23, 24, 25},
	},
	"ReplayedLater": {
		128, 0x0000FFFFFFFFFFFF,
		[]uint64{16, 32, 48, 64, 80, 96, 112, 128, 16, 32, 48, 64, 80, 96, 112, 128},
		[]bool{
			true, true, true, true, true, true, true, true, false, false,
			false, false, false, false, false, false,
		},
		[]bool{
			true, true, true, true, true, true, true, true, false, false,
			false, false, false, false, false, false,
		},
		[]uint64{16, 32, 48, 64, 80, 96, 112, 128},
	},
	"ReplayedQuick": {
		128, 0x0000FFFFFFFFFFFF,
		[]uint64{16, 16, 32, 32, 48, 48, 64, 64, 80, 80, 96, 96, 112, 112, 128, 128},
		[]bool{
			true, false, true, false, true, false, true, false, true, false,
			true, false, true, false, true, false,
		},
		[]bool{
			true, false, true, false, true, false, true, false, true, false,
			true, false, true, false, true, false,
		},
		[]uint64{16, 32, 48, 64, 80, 96, 112, 128},
	},
	"Strict": {
		0, 0x0000FFFFFFFFFFFF,
		[]uint64{1, 3, 2, 4, 5, 6, 7, 8, 9, 10},
		[]bool{
			true, true, false, true, true, true, true, true, true, true,
		},
		[]bool{
			true, true, false, true, true, true, true, true, true, true,
		},
		[]uint64{1, 3, 4, 5, 6, 7, 8, 9, 10},
	},
	"Overflow": {
		128, 0x0000FFFFFFFFFFFF,
		[]uint64{0x0000FFFFFFFFFFFE, 0x0000FFFFFFFFFFFF, 0x0001000000000000, 0x0001000000000001},
		[]bool{
			true, true, false, false,
		},
		[]bool{
			true, true, false, false,
		},
		[]uint64{0x0000FFFFFFFFFFFE, 0x0000FFFFFFFFFFFF},
	},
}

func runCheckCase(t *testing.T, det ReplayDetector, tc testCase) {
	t.Helper()

	var out []uint64
	for i, seq := range tc.input {
		accept, ok := det.Check(seq)
		assert.Equal(t, tc.valid[i], ok, "Unexpected validity")
		if ok {
			out = append(out, seq)
		}
		assert.Equal(t, tc.latest[i], accept(), "Unexpected sequence latest status")
	}
	assert.Equal(t, tc.expected, out, "Wrong replay detection result")
}

func runCheckAccepterCase(t *testing.T, det ReplayDetector, tc testCase) {
	t.Helper()

	ca, ok := det.(CheckAccepter)
	assert.True(t, ok, "detector must implement CheckAccepter")

	var out []uint64
	for i, seq := range tc.input {
		tok := ca.CheckSeq(seq)
		assert.Equal(t, tc.valid[i], tok.Passed(), "Unexpected validity")
		if tok.Passed() {
			out = append(out, seq)
		}
		assert.Equal(t, tc.latest[i], ca.Accept(tok), "Unexpected sequence latest status")
	}
	assert.Equal(t, tc.expected, out, "Wrong replay detection result")
}

func TestReplayDetector(t *testing.T) {
	for apiName, runCase := range map[string]func(*testing.T, ReplayDetector, testCase){
		"Check":         runCheckCase,
		"CheckAccepter": runCheckAccepterCase,
	} {
		t.Run(apiName, func(t *testing.T) {
			for name, tc := range commonCases {
				t.Run(name, func(t *testing.T) {
					runCase(t, New(tc.windowSize, tc.maxSeq), tc)
				})
			}
		})
	}
}

func TestReplayDetectorWrapped(t *testing.T) {
	cases := map[string]testCase{
		"WrapContinuous": {
			64, 0xFFFF,
			[]uint64{0xFFFC, 0xFFFD, 0xFFFE, 0xFFFF, 0x0000, 0x0001, 0x0002, 0x0003},
			[]bool{
				true, true, true, true, true, true, true, true,
			},
			[]bool{
				true, true, true, true, true, true, true, true,
			},
			[]uint64{0xFFFC, 0xFFFD, 0xFFFE, 0xFFFF, 0x0000, 0x0001, 0x0002, 0x0003},
		},
		"WrapReordered": {
			64, 0xFFFF,
			[]uint64{0xFFFD, 0xFFFC, 0x0002, 0xFFFE, 0x0000, 0x0001, 0xFFFF, 0x0003},
			[]bool{
				true, true, true, true, true, true, true, true,
			},
			[]bool{
				true, false, true, false, false, false, false, true,
			},
			[]uint64{0xFFFD, 0xFFFC, 0x0002, 0xFFFE, 0x0000, 0x0001, 0xFFFF, 0x0003},
		},
		"WrapReorderedReplayed": {
			64, 0xFFFF,
			[]uint64{0xFFFD, 0xFFFC, 0xFFFC, 0x0002, 0xFFFE, 0xFFFC, 0x0000, 0x0001, 0x0001, 0xFFFF, 0x0001, 0x0003},
			[]bool{
				true, true, false, true, true, false, true, true, false, true, false, true,
			},
			[]bool{
				true, false, false, true, false, false, false, false, false, false, false, true,
			},
			[]uint64{0xFFFD, 0xFFFC, 0x0002, 0xFFFE, 0x0000, 0x0001, 0xFFFF, 0x0003},
		},
		"BeforeWrapReplayed": {
			64, 0xFFFF,
			[]uint64{0x0, 0xFFFF, 0xFFFF},
			[]bool{
				true, true, false,
			},
			[]bool{
				true, false, false,
			},
			[]uint64{0x0, 0xFFFF},
		},
	}
	for name, c := range commonCases {
		_, ok := cases[name]
		assert.False(t, ok, "Duplicate test case name: %q", name)
		cases[name] = c
	}

	for apiName, runCase := range map[string]func(*testing.T, ReplayDetector, testCase){
		"Check":         runCheckCase,
		"CheckAccepter": runCheckAccepterCase,
	} {
		t.Run(apiName, func(t *testing.T) {
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) {
					runCase(t, WithWrap(tc.windowSize, tc.maxSeq), tc)
				})
			}
		})
	}
}

// TestCheckSeqDoesNotCommit verifies that CheckSeq alone does not update the
// replay list: RFC 3711 Section 3.3.2 requires the list to be updated only
// after the packet has been authenticated, so a sequence number that passed
// CheckSeq but was never accepted must still pass a later check.
func TestCheckSeqDoesNotCommit(t *testing.T) {
	kinds := map[string]ReplayDetector{
		"New":      New(16, 0xFFFF),
		"WithWrap": WithWrap(16, 0xFFFF),
	}

	for name, detector := range kinds {
		t.Run(name, func(t *testing.T) {
			det, ok := detector.(CheckAccepter)
			assert.True(t, ok, "detector must implement CheckAccepter")

			assert.True(t, det.CheckSeq(5).Passed(), "first check must pass")
			tok := det.CheckSeq(5)
			assert.True(t, tok.Passed(), "re-check of an unaccepted seq must pass")
			det.Accept(tok)
			assert.False(t, det.CheckSeq(5).Passed(), "check after accept must reject the duplicate")
		})
	}
}

// TestRejectedTokenIsInert verifies that accepting a rejected or zero-value
// Token does not modify the window: a sequence number that failed its check
// cannot corrupt the detector even if its Token is accepted.
func TestRejectedTokenIsInert(t *testing.T) {
	kinds := map[string]ReplayDetector{
		"New":      New(16, 0xFFFF),
		"WithWrap": WithWrap(16, 0xFFFF),
	}

	for name, detector := range kinds {
		t.Run(name, func(t *testing.T) {
			det, ok := detector.(CheckAccepter)
			assert.True(t, ok, "detector must implement CheckAccepter")

			rejected := det.CheckSeq(0x10000) // beyond maxSeq
			assert.False(t, rejected.Passed(), "out-of-range seq must fail the check")
			assert.False(t, det.Accept(rejected), "accepting a rejected token must do nothing")
			assert.False(t, det.Accept(Token{}), "accepting a zero-value token must do nothing")

			tok := det.CheckSeq(5)
			assert.True(t, tok.Passed(), "window must be intact after inert accepts")
			det.Accept(tok)
			assert.False(t, det.CheckSeq(5).Passed(), "duplicate must still be rejected after inert accepts")
		})
	}
}

func TestCheckAccepterNoAllocation(t *testing.T) {
	kinds := map[string]ReplayDetector{
		"New":      New(128, 0x0000FFFFFFFFFFFF),
		"WithWrap": WithWrap(128, 0xFFFF),
	}

	for name, detector := range kinds {
		t.Run(name, func(t *testing.T) {
			det, ok := detector.(CheckAccepter)
			assert.True(t, ok, "detector must implement CheckAccepter")

			seq := uint64(0)
			allocs := testing.AllocsPerRun(1000, func() {
				seq++
				det.Accept(det.CheckSeq(seq))
			})
			assert.Zero(t, allocs, "CheckSeq/Accept must not allocate")
		})
	}
}
