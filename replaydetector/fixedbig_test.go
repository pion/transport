// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package replaydetector

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Example_fixedBigInt_SetBit() {
	bi := newFixedBigInt(224)

	bi.SetBit(0)
	fmt.Println(bi.String())
	bi.Lsh(1)
	fmt.Println(bi.String())

	bi.Lsh(0)
	fmt.Println(bi.String())

	bi.SetBit(10)
	fmt.Println(bi.String())
	bi.Lsh(20)
	fmt.Println(bi.String())

	bi.SetBit(80)
	fmt.Println(bi.String())
	bi.Lsh(4)
	fmt.Println(bi.String())

	bi.SetBit(130)
	fmt.Println(bi.String())
	bi.Lsh(64)
	fmt.Println(bi.String())

	bi.SetBit(7)
	fmt.Println(bi.String())

	bi.Lsh(129)
	fmt.Println(bi.String())

	for range 256 {
		bi.Lsh(1)
		bi.SetBit(0)
	}
	fmt.Println(bi.String())

	// output:
	// 0000000000000000000000000000000000000000000000000000000000000001
	// 0000000000000000000000000000000000000000000000000000000000000002
	// 0000000000000000000000000000000000000000000000000000000000000002
	// 0000000000000000000000000000000000000000000000000000000000000402
	// 0000000000000000000000000000000000000000000000000000000040200000
	// 0000000000000000000000000000000000000000000100000000000040200000
	// 0000000000000000000000000000000000000000001000000000000402000000
	// 0000000000000000000000000000000400000000001000000000000402000000
	// 0000000000000004000000000010000000000004020000000000000000000000
	// 0000000000000004000000000010000000000004020000000000000000000080
	// 0000000004000000000000000000010000000000000000000000000000000000
	// 00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF
}

func TestFixedBigIntShiftAcrossWindow(t *testing.T) {
	for _, width := range []uint{1, 63, 64, 65, 96, 127, 128} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			value := newFixedBigInt(width)
			value.SetBit(0)
			for position := range width {
				require.Equal(t, uint(1), value.Bit(position), "position %d", position)
				value.Lsh(1)
			}
			for _, word := range value.bits {
				require.Zero(t, word, "bits shifted beyond the window must be cleared")
			}
		})
	}
}
