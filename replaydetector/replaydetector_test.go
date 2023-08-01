// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package replaydetector

import (
	"reflect"
	"testing"
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

func TestReplayDetector(t *testing.T) {
	for name, c := range commonCases {
		c := c
		t.Run(name, func(t *testing.T) {
			det := New(c.windowSize, c.maxSeq)
			var out []uint64
			for i, seq := range c.input {
				accept, ok := det.Check(seq)
				if ok != c.valid[i] {
					t.Errorf("Unexpected validity (%d):\nexpected: %v\ngot: %v", seq, c.valid[i], ok)
				}
				if ok {
					out = append(out, seq)
				}
				if latest := accept(); latest != c.latest[i] {
					t.Errorf("Unexpected sequence latest status (%d):\nexpected: %v\ngot: %v", seq, c.latest[i], latest)
				}
			}
			if !reflect.DeepEqual(c.expected, out) {
				t.Errorf("Wrong replay detection result:\nexpected: %v\ngot:      %v",
					c.expected, out,
				)
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
	}
	for name, c := range commonCases {
		if _, ok := cases[name]; ok {
			t.Fatalf("Duplicate test case name: %q", name)
		}
		cases[name] = c
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			det := WithWrap(c.windowSize, c.maxSeq)
			var out []uint64
			for i, seq := range c.input {
				accept, ok := det.Check(seq)
				if ok != c.valid[i] {
					t.Errorf("Unexpected validity (%d):\nexpected: %v\ngot: %v", seq, c.valid[i], ok)
				}
				if ok {
					out = append(out, seq)
				}
				if latest := accept(); latest != c.latest[i] {
					t.Errorf("Unexpected sequence latest status (%d):\nexpected: %v\ngot: %v", seq, c.latest[i], latest)
				}
			}
			if !reflect.DeepEqual(c.expected, out) {
				t.Errorf("Wrong replay detection result:\nexpected: %v\ngot:      %v",
					c.expected, out,
				)
			}
		})
	}
}
