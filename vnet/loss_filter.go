// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type LossFilterHandler interface {
	shouldDrop() bool
	setLossRate(chance int, resetImmediately bool)
}

// LossFilter is a wrapper around NICs, that drops some of the packets passed to
// onInboundChunk.
type LossFilter struct {
	NIC
	LossFilterHandler
}

// RandomLossHandler drops packets randomly with a probability determined by the chance parameter.
type RandomLossHandler struct {
	chance int
	mutex  sync.RWMutex
}

// NewRandomLossHandler creates a new RandomLossHandler with the given drop chance.
func NewRandomLossHandler(chance int) (*RandomLossHandler, error) {
	if !validateChance(chance) {
		return nil, fmt.Errorf("chance must be between 0 and 100 inclusive")
	}

	return &RandomLossHandler{
		chance: chance,
	}, nil
}

func (r *RandomLossHandler) shouldDrop() bool {
	r.mutex.RLock()
	chance := r.chance
	r.mutex.RUnlock()
	return rand.Intn(100) < chance
}

func (r *RandomLossHandler) setLossRate(chance int, resetImmediately bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.chance = chance
}

// RandomShuffleLossHandler drops packets with a deterministic probability for every 100 packets
// That is, for every 100 packets, it guarentees that the number of packets dropped is equal to the chance parameter.
type RandomShuffleLossHandler struct {
	blockIdx      int
	shuffledBlock []bool
	currentChance int
	pendingChance int
	mutex         sync.Mutex
}

// NewRandomShuffleLossHandler creates a new RandomShuffleLossHandler with the given drop chance and shuffle block size.
// The default shuffle block size should be 100.
func NewRandomShuffleLossHandler(chance int, shuffleBlockSize int) (*RandomShuffleLossHandler, error) {

	if !validateChance(chance) {
		return nil, fmt.Errorf("chance must be between 0 and 100 inclusive")
	}

	if shuffleBlockSize < 1 {
		return nil, fmt.Errorf("shuffleBlockSize must be greater than 0")
	}

	filter := RandomShuffleLossHandler{
		shuffledBlock: make([]bool, shuffleBlockSize),
		blockIdx:      0,
		currentChance: chance,
		pendingChance: chance,
	}

	for i := 0; i < filter.currentChance; i++ {
		filter.shuffledBlock[i] = true
	}

	filter.shuffleBlock()
	return &filter, nil
}

func (r *RandomShuffleLossHandler) setLossRate(chance int, resetImmediately bool) {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.pendingChance = chance

	if resetImmediately {
		r.shuffleBlock()
	}
}

func (r *RandomShuffleLossHandler) shuffleBlock() {

	for i := 0; i < len(r.shuffledBlock); i++ {
		if r.pendingChance == r.currentChance {
			break
		} else if r.pendingChance > r.currentChance && !r.shuffledBlock[i] {
			r.shuffledBlock[i] = true
			r.currentChance++
		} else if r.pendingChance < r.currentChance && r.shuffledBlock[i] {
			r.shuffledBlock[i] = false
			r.currentChance--
		}
	}

	rand.Shuffle(len(r.shuffledBlock), func(i, j int) {
		r.shuffledBlock[i], r.shuffledBlock[j] = r.shuffledBlock[j], r.shuffledBlock[i]
	})
	r.blockIdx = 0
}

func (r *RandomShuffleLossHandler) shouldDrop() bool {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.blockIdx == len(r.shuffledBlock) {
		r.shuffleBlock()
	}

	res := r.shuffledBlock[r.blockIdx]
	r.blockIdx++
	return res
}

// NewLossFilter creates a new LossFilter that drops every packet with a
// probability of chance/100 by default. You can provide a custom handler to
// override the default behavior.
func NewLossFilter(nic NIC, chance int, handler ...LossFilterHandler) (*LossFilter, error) {

	var lossHandler LossFilterHandler
	var err error

	if !validateChance(chance) {
		return nil, fmt.Errorf("chance must be between 0 and 100 inclusive")
	}

	if len(handler) > 0 {
		lossHandler = handler[0]
	} else {
		lossHandler, err = NewRandomLossHandler(chance)
		if err != nil {
			return nil, err
		}
	}

	f := &LossFilter{
		NIC:               nic,
		LossFilterHandler: lossHandler,
	}
	//nolint:staticcheck
	rand.Seed(time.Now().UTC().UnixNano())

	f.LossFilterHandler.setLossRate(chance, false)
	return f, nil
}

func (f *LossFilter) onInboundChunk(c Chunk) {

	if f.LossFilterHandler.shouldDrop() {
		return
	}

	f.NIC.onInboundChunk(c)
}

// SetLossRate sets the loss rate for the loss filter.
// The chance parameter is an integer out of 100.
// The resetImmediately parameter is a boolean that indicates whether to reset the loss rate immediately.
// If resetImmediately is true, the loss rate will be reset immediately.
// If resetImmediately is false, the loss rate will be reset after the next shuffle for RandomShuffleLossHandler
// Note that for random loss handler, the loss rate will be reset immediately regardless of the resetImmediately parameter.
func (f *LossFilter) SetLossRate(chance int, resetImmediately bool) error {
	if !validateChance(chance) {
		return fmt.Errorf("chance must be between 0 and 100 inclusive")
	}

	f.LossFilterHandler.setLossRate(chance, resetImmediately)
	return nil
}

func validateChance(chance int) bool {
	return chance >= 0 && chance <= 100
}
