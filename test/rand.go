package test

import (
	crand "crypto/rand"
	"fmt"
	mrand "math/rand"
)

var randomness []byte

func init() {
	// read 1MB of randomness
	randomness = make([]byte, 1<<20)
	for i := 0; i < len(randomness); i += 65536 {
		// the number of bytes of entropy is limited by 65536 on some js environment
		if _, err := crand.Read(randomness[i : i+65536]); err != nil {
			fmt.Println("Failed to initiate randomness:", err)
		}
	}
}

func randBuf(size int) ([]byte, error) {
	n := len(randomness) - size
	if n < 1 {
		return nil, fmt.Errorf("requested too large buffer (%d). max is %d", size, len(randomness))
	}

	start := mrand.Intn(n)
	return randomness[start : start+size], nil
}
