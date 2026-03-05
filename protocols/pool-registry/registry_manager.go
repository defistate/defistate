package poolregistry

import (
	"errors"
	"fmt"
	"sync"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
)

var (
	// ErrTokenNotFound is returned when a token is not found in the token system.
	ErrTokenNotFound = errors.New("token not found")
)

// RegistryManager handles the business logic of creating and deleting core entities
// across the various protocol-agnostic systems by depending on interfaces.
type RegistryManager struct {
	tokenSystem            TokenSystemInterface
	poolSystem             PoolSystemInterface
	tokenPoolSystem        TokenPoolSystemInterface
	beforeRegisterPoolFunc func(PoolKey)
	mu                     sync.RWMutex
}

// NewRegistryManager creates a new manager for core entities.
func NewRegistryManager(
	tokenSystem TokenSystemInterface,
	poolSystem PoolSystemInterface,
	tokenPoolSystem TokenPoolSystemInterface,
) *RegistryManager {
	return &RegistryManager{
		tokenSystem:     tokenSystem,
		poolSystem:      poolSystem,
		tokenPoolSystem: tokenPoolSystem,
	}
}

// RegisterPool provides a single entry point to register a new pool and its associated tokens.
func (rm *RegistryManager) RegisterPool(
	poolKey PoolKey,
	tokenAddrs []common.Address,
	protocolID engine.ProtocolID,
) (uint64, []uint64, error) {

	rm.mu.RLock()
	beforeRegisterPoolFunc := rm.beforeRegisterPoolFunc
	rm.mu.RUnlock()

	if beforeRegisterPoolFunc != nil {
		beforeRegisterPoolFunc(poolKey)
	}

	// Step 1: Look up each token to ensure it exists and get its ID.
	tokenIDs := make([]uint64, len(tokenAddrs))
	for i, tokenAddr := range tokenAddrs {
		existingToken, err := rm.tokenSystem.GetTokenByAddress(tokenAddr)
		if err != nil {
			return 0, nil, ErrTokenNotFound
		}
		tokenIDs[i] = existingToken.ID
	}

	// Step 2: Add the pool to the registry using the Key and ProtocolID.
	poolID, err := rm.poolSystem.AddPool(poolKey, protocolID)
	if err != nil {
		return 0, nil, err
	}

	// Step 3: Link the tokens to the newly created pool.
	rm.tokenPoolSystem.AddPool(tokenIDs, poolID)

	return poolID, tokenIDs, nil
}

// RegisterPools provides a single entry point to register a batch of new pools.
// This is a "best-effort" operation that attempts to register all valid pools
// and returns slices for the new pool IDs, their corresponding token IDs, and any errors.
func (rm *RegistryManager) RegisterPools(
	poolKeys []PoolKey,
	tokenAddrSets [][]common.Address,
	protocolIDs []engine.ProtocolID,
) ([]uint64, [][]uint64, []error) {
	rm.mu.RLock()
	beforeRegisterPoolFunc := rm.beforeRegisterPoolFunc
	rm.mu.RUnlock()

	if beforeRegisterPoolFunc != nil {
		for _, pk := range poolKeys {
			beforeRegisterPoolFunc(pk)
		}
	}

	if len(poolKeys) != len(tokenAddrSets) || len(poolKeys) != len(protocolIDs) {
		panic("mismatched input slice lengths in RegisterPools")
	}

	finalPoolIDs := make([]uint64, len(poolKeys))
	finalTokenIDSets := make([][]uint64, len(poolKeys))
	finalErrs := make([]error, len(poolKeys))
	var hasErrors bool

	// Step 1: Pre-flight check. Validate all unique tokens required by the batch.
	uniqueTokenAddrs := make(map[common.Address]struct{})
	for _, set := range tokenAddrSets {
		for _, addr := range set {
			uniqueTokenAddrs[addr] = struct{}{}
		}
	}

	//
	// Visualizing the mapping of token addresses to IDs before pool creation.

	tokenIDMap := make(map[common.Address]uint64, len(uniqueTokenAddrs))
	missingTokens := make(map[common.Address]struct{})

	for addr := range uniqueTokenAddrs {
		t, err := rm.tokenSystem.GetTokenByAddress(addr)
		if err != nil {
			missingTokens[addr] = struct{}{}
		} else {
			tokenIDMap[addr] = t.ID
		}
	}

	// Step 2: Prepare the batch for the pools that are actually valid.
	// We filter out any pools that depend on a missing token.
	var validPoolKeys []PoolKey
	var validProtocols []engine.ProtocolID
	var validOriginalIndices []int // Tracks the original index of each valid pool.

	for i, tokenSet := range tokenAddrSets {
		isValid := true
		for _, tokenAddr := range tokenSet {
			if _, isMissing := missingTokens[tokenAddr]; isMissing {
				finalErrs[i] = fmt.Errorf("pre-flight check failed, token not found: %s", tokenAddr.Hex())
				hasErrors = true
				isValid = false
				break
			}
		}
		if isValid {
			validPoolKeys = append(validPoolKeys, poolKeys[i])
			validProtocols = append(validProtocols, protocolIDs[i])
			validOriginalIndices = append(validOriginalIndices, i)
		}
	}

	// If no pools are valid after the token check, we can return early.
	if len(validPoolKeys) == 0 {
		if hasErrors {
			return finalPoolIDs, finalTokenIDSets, finalErrs
		}
		return nil, nil, nil
	}

	// Step 3: Add all valid pools to the pool registry in a single batch.
	// We pass the filtered valid lists to the underlying system.
	newPoolIDs, addErrs := rm.poolSystem.AddPools(validPoolKeys, validProtocols)
	for i, err := range addErrs {
		if err != nil {
			originalIndex := validOriginalIndices[i]
			finalErrs[originalIndex] = err
			hasErrors = true
		}
	}

	// Step 4: Prepare data for linking, but only for pools that were successfully added.
	var successfulPoolIDs []uint64
	var successfulTokenIDSets [][]uint64

	for i, poolID := range newPoolIDs {
		originalIndex := validOriginalIndices[i]

		// If an error occurred for this pool at any previous step, skip it.
		if finalErrs[originalIndex] != nil {
			continue
		}
		if poolID == 0 {
			finalErrs[originalIndex] = fmt.Errorf("consistency error: pool %s was not registered but returned no error", poolKeys[originalIndex].String())
			hasErrors = true
			continue
		}

		finalPoolIDs[originalIndex] = poolID // Store the successful ID in the final result.
		successfulPoolIDs = append(successfulPoolIDs, poolID)

		// Map token addresses to their IDs for the token-pool system.
		tokenIDs := make([]uint64, len(tokenAddrSets[originalIndex]))
		for j, tokenAddr := range tokenAddrSets[originalIndex] {
			tokenIDs[j] = tokenIDMap[tokenAddr]
		}
		successfulTokenIDSets = append(successfulTokenIDSets, tokenIDs)
		finalTokenIDSets[originalIndex] = tokenIDs
	}

	// Step 5: Link the successfully created pools in the token-pool system.
	if len(successfulPoolIDs) > 0 {
		rm.tokenPoolSystem.AddPools(successfulPoolIDs, successfulTokenIDSets)
	}

	if hasErrors {
		return finalPoolIDs, finalTokenIDSets, finalErrs
	}
	return finalPoolIDs, finalTokenIDSets, nil
}

// DeletePool removes a pool from all relevant core systems.
func (rm *RegistryManager) DeletePool(poolID uint64) error {
	err := rm.poolSystem.DeletePool(poolID)
	if err != nil {
		return err
	}
	rm.tokenPoolSystem.RemovePool(poolID)
	return nil
}

// DeletePools removes a batch of pools from all core systems.
func (rm *RegistryManager) DeletePools(poolIDs []uint64) []error {
	// First, remove all connections from the graph-based system.
	rm.tokenPoolSystem.RemovePools(poolIDs)
	// Then, remove the pools from the primary pool registry and return any errors.
	return rm.poolSystem.DeletePools(poolIDs)
}

func (rm *RegistryManager) SetBeforeRegisterPoolFunc(f func(PoolKey)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.beforeRegisterPoolFunc = f
}
