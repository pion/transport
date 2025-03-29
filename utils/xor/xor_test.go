// SPDX-FileCopyrightText: 2013 The Go Authors. All rights reserved.
// SPDX-License-Identifier: BSD-3-Clause

package xor

import (
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestXOR(t *testing.T) { //nolint:cyclop
	for j := 1; j <= 1024; j++ { //nolint:varnamelen
		if testing.Short() && j > 16 {
			break
		}
		for alignP := 0; alignP < 2; alignP++ {
			for alignQ := 0; alignQ < 2; alignQ++ {
				for alignD := 0; alignD < 2; alignD++ {
					p := make([]byte, j)[alignP:] //nolint:varnamelen
					q := make([]byte, j)[alignQ:] //nolint:varnamelen
					d0 := make([]byte, j+alignD+1)
					d0[j+alignD] = 42
					d1 := d0[alignD : j+alignD]
					d2 := make([]byte, j+alignD)[alignD:]
					_, err := io.ReadFull(rand.Reader, p)
					assert.NoError(t, err)
					_, err = io.ReadFull(rand.Reader, q)
					assert.NoError(t, err)
					XorBytes(d1, p, q)
					n := minInt(p, q)
					for i := 0; i < n; i++ {
						d2[i] = p[i] ^ q[i]
					}
					assert.Equalf(t, d1, d2, "p: %#v, q: %#v", p, q)
					assert.Equal(t, byte(42), d0[j+alignD], "guard overwritten")
				}
			}
		}
	}
}

func minInt(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	return n
}

func BenchmarkXORAligned(b *testing.B) {
	dst := make([]byte, 1<<15)
	data0 := make([]byte, 1<<15)
	data1 := make([]byte, 1<<15)
	sizes := []int64{1 << 3, 1 << 7, 1 << 11, 1 << 15}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dBytes", size), func(b *testing.B) {
			s0 := data0[:size]
			s1 := data1[:size]
			b.SetBytes(size)
			for i := 0; i < b.N; i++ {
				XorBytes(dst, s0, s1)
			}
		})
	}
}

func BenchmarkXORUnalignedDst(b *testing.B) {
	dst := make([]byte, 1<<15+1)
	data0 := make([]byte, 1<<15)
	data1 := make([]byte, 1<<15)
	sizes := []int64{1 << 3, 1 << 7, 1 << 11, 1 << 15}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dBytes", size), func(b *testing.B) {
			s0 := data0[:size]
			s1 := data1[:size]
			b.SetBytes(size)
			for i := 0; i < b.N; i++ {
				XorBytes(dst[1:], s0, s1)
			}
		})
	}
}
