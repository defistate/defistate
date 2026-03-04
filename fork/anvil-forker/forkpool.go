package fork

import (
	"context"
	"fmt"
	"sync/atomic"
)

type ForkPoolConfig []ForkConfig

type ForkPool struct {
	forks     []*Fork
	nextIndex atomic.Uint32
}

func NewForkPool(ctx context.Context, cfg ForkPoolConfig) (*ForkPool, error) {
	// 1. Validate that size is non-zero and matches the slice length
	if len(cfg) == 0 {
		return nil, fmt.Errorf("fork pool size must be greater than zero")
	}

	pool := &ForkPool{
		forks: make([]*Fork, len(cfg)),
	}

	// 2. Initialize each fork using its explicit configuration
	for i, forkCfg := range cfg {
		// The caller has total control over ports, channels, and overrides for each index
		pool.forks[i] = NewFork(ctx, forkCfg)
	}

	return pool, nil
}

func (p *ForkPool) Get() *Fork {
	// Round-robin distribution
	idx := p.nextIndex.Add(1) % uint32(len(p.forks))
	return p.forks[idx]
}
