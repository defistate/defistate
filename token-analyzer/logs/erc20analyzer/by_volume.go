package erc20analyzer

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/defistate/defistate/token-analyzer/erc20analyzer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Config holds the configuration parameters for the VolumeAnalyzer.
type Config struct {
	// ExpiryCheckFrequency is the interval at which the analyzer checks for stale records.
	ExpiryCheckFrequency time.Duration
	// RecordStaleDuration is the duration after which a record is considered stale and pruned.
	RecordStaleDuration time.Duration
	// IsAllowedAddress is a function to filter which addresses are included in the analysis.
	IsAllowedAddress func(common.Address) bool
}

func (cfg Config) validate() error {
	if cfg.ExpiryCheckFrequency <= 0 {
		return errors.New("ExpiryCheckFrequency cannot be <= 0")
	}
	if cfg.RecordStaleDuration <= 0 {
		return errors.New("RecordStaleDuration cannot be <= 0")
	}
	if cfg.IsAllowedAddress == nil {
		return errors.New("IsAllowedAddress cannot be nil")
	}

	return nil
}

// VolumeAnalyzer is an implementation of TokenHolderAnalyzer that determines
// the primary holder based on the highest total transfer volume.
type VolumeAnalyzer struct {
	mu               sync.RWMutex
	records          map[common.Address]MaxTransferRecord
	staleDuration    time.Duration
	checkTicker      *time.Ticker
	isAllowedAddress func(common.Address) bool
}

// ensures we implement the correct interface
var _ erc20analyzer.TokenHolderAnalyzer = (*VolumeAnalyzer)(nil)

// NewVolumeAnalyzer creates and starts a new instance of the volume-based analyzer.
func NewVolumeAnalyzer(ctx context.Context, cfg Config) (*VolumeAnalyzer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	a := &VolumeAnalyzer{
		records:          make(map[common.Address]MaxTransferRecord),
		staleDuration:    cfg.RecordStaleDuration,
		checkTicker:      time.NewTicker(cfg.ExpiryCheckFrequency),
		isAllowedAddress: cfg.IsAllowedAddress,
	}

	// The ticker is stopped automatically when the context is done.
	go a.startExpiryTicker(ctx)

	return a, nil
}

// Update processes logs to find the highest total volume transferrers and updates the state.
func (a *VolumeAnalyzer) Update(logs []types.Log) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(logs) == 0 {
		return
	}

	// Extract the max total volume transferrers from the logs.
	// @todo this function should be passed as a dependency to allow for better flexibility
	// using ExtractMaxTotalVolumeTransferrer will enable faster token detection (but with a probabilty that records might no longer hold tokens )
	// using ExtractMaxTotalVolumeReceiver gives a higher probability that records will hold tokens, but depending on a.isAllowedAddress, it might detect tokens slower
	newRecords := ExtractMaxTotalVolumeTransferrer(logs, a.isAllowedAddress)
	if len(newRecords) == 0 {
		return
	}

	a.records = MergeMaxTransferRecords(a.records, newRecords)
}

// TokenByMaxKnownHolder returns the current primary holders based on total volume.
func (a *VolumeAnalyzer) TokenByMaxKnownHolder() map[common.Address]common.Address {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[common.Address]common.Address, len(a.records))
	for token, record := range a.records {
		result[token] = record.Address
	}
	return result
}

// RecordByToken returns the MaxTransferRecord for a given token address.
// It returns the record and a boolean indicating if the token was found.
func (a *VolumeAnalyzer) RecordByToken(token common.Address) (MaxTransferRecord, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	record, ok := a.records[token]
	if !ok {
		return MaxTransferRecord{}, false
	}

	recordCpy := record
	recordCpy.Amount = new(big.Int).Set(record.Amount)
	return recordCpy, ok
}

// startExpiryTicker runs the background process to prune stale records.
// It terminates when the provided context is canceled.
func (a *VolumeAnalyzer) startExpiryTicker(ctx context.Context) {
	defer a.checkTicker.Stop()
	for {
		select {
		case <-a.checkTicker.C:
			a.resetStaleRecords()
		case <-ctx.Done():
			return
		}
	}
}

// resetStaleRecords performs the actual pruning of stale data.
//
// Problem:
// The current implementation creates gaps for which we would have zero records for all tokens
// This is unacceptable
// If we provided holders for a token in the past, we should try our possible best to provide at least
// the same holders in the future
//
// Solution:
// Instead of simply deleting records, we set the max transfer record Amount to zero
// This ensures that we still retain holders for tokens with no current activity
func (a *VolumeAnalyzer) resetStaleRecords() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for token, record := range a.records {
		// expire by resetting Amount to zero
		// this ensures that we always have a potentially valid holder
		// even though a token has no activity for a while
		if time.Since(record.Time) > a.staleDuration {
			a.records[token] = MaxTransferRecord{
				Address: record.Address,             // keep address
				Amount:  record.Amount.SetUint64(0), // reuse big.Int
				Time:    now,                        // set new time
			}
		}
	}
}
