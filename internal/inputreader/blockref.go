package main

import (
	"fmt"
	"math/big"
	"sync"
)

type BlockRef struct {
	mutex  sync.Mutex
	number *big.Int
}

func (b *BlockRef) Set(number *big.Int) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.number == nil || b.number.Cmp(number) < 0 {
		b.number = number
		return nil
	}
	return fmt.Errorf("cannot set a number lower than %v", b.number)
}

func (b *BlockRef) Number() *big.Int {
	return b.number
}
