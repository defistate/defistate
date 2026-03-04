package pancakeswap

import (
	"math/big"
	"testing"

	aerodromeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helper Functions ---

// createSwapLog is a helper function to construct a valid Uniswap V3 Swap logger for testing.
// It packs the non-indexed arguments into the logger's Data field using the ABI.
func createSwapLog(t *testing.T, poolAddress common.Address, tick int64, liquidity, sqrtPrice *big.Int) types.Log {
	// The non-indexed arguments for a Swap event are:
	// int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick
	// We'll use placeholder values for amount0 and amount1.
	amount0 := big.NewInt(100)
	amount1 := big.NewInt(200)

	// Get the swap event from the ABI
	swapEvent, ok := aerodromeabi.AerodromeABI.Events["Swap"]
	require.True(t, ok, "Swap event not found in ABI")

	// Correctly pack the non-indexed arguments for the event logger's Data field.
	// We filter the event inputs to get only the non-indexed ones.
	var nonIndexedArgs abi.Arguments
	for _, arg := range swapEvent.Inputs {
		if !arg.Indexed {
			nonIndexedArgs = append(nonIndexedArgs, arg)
		}
	}

	// The ABI packer expects a *big.Int for the 'int24' tick type, not an int64.
	tickAsBigInt := big.NewInt(tick)

	// Pack the arguments into the data field using the filtered non-indexed argument definitions.
	packedData, err := nonIndexedArgs.Pack(amount0, amount1, sqrtPrice, liquidity, tickAsBigInt)
	require.NoError(t, err, "Failed to pack swap event data")

	return types.Log{
		Address: poolAddress,
		Topics:  []common.Hash{swapEvent.ID},
		Data:    packedData,
	}
}

func TestExtractSwapsFromLogs(t *testing.T) {
	t.Run("successfully extracts data from valid logs with positive and negative ticks", func(t *testing.T) {
		// --- Setup ---
		pool1 := common.HexToAddress("0x1")
		pool2 := common.HexToAddress("0x2")

		data1 := struct {
			tick      int64
			liquidity *big.Int
			sqrtPrice *big.Int
		}{198750, big.NewInt(1000), big.NewInt(1)} // Positive tick

		data2 := struct {
			tick      int64
			liquidity *big.Int
			sqrtPrice *big.Int
		}{-887272, big.NewInt(2000), big.NewInt(2)} // Negative tick

		logs := []types.Log{
			createSwapLog(t, pool1, data1.tick, data1.liquidity, data1.sqrtPrice),
			{Address: common.HexToAddress("0x3"), Topics: []common.Hash{common.HexToHash("0x1234")}}, // Irrelevant logger
			createSwapLog(t, pool2, data2.tick, data2.liquidity, data2.sqrtPrice),
		}

		// --- Action ---
		pools, ticks, liquidities, sqrtPricesX96, err := ExtractSwapsFromLogs(logs)

		// --- Assertions ---
		require.NoError(t, err)
		require.Len(t, pools, 2, "Should only find 2 swap events")
		require.Len(t, ticks, 2)
		require.Len(t, liquidities, 2)
		require.Len(t, sqrtPricesX96, 2)

		// Check data for the first swap logger (positive tick)
		assert.Equal(t, pool1, pools[0])
		assert.Equal(t, data1.tick, ticks[0])
		assert.Equal(t, 0, data1.liquidity.Cmp(liquidities[0]), "Liquidity for pool 1 should match")
		assert.Equal(t, 0, data1.sqrtPrice.Cmp(sqrtPricesX96[0]), "SqrtPrice for pool 1 should match")

		// Check data for the second swap logger (negative tick)
		assert.Equal(t, pool2, pools[1])
		assert.Equal(t, data2.tick, ticks[1])
		assert.Equal(t, 0, data2.liquidity.Cmp(liquidities[1]), "Liquidity for pool 2 should match")
		assert.Equal(t, 0, data2.sqrtPrice.Cmp(sqrtPricesX96[1]), "SqrtPrice for pool 2 should match")
	})

	t.Run("Returns error on malformed logger data", func(t *testing.T) {
		// --- Setup ---
		pool1 := common.HexToAddress("0x1")
		swapEventID := aerodromeabi.AerodromeABI.Events["Swap"].ID

		// Create a logger with the correct topic but corrupted/short data field.
		malformedLog := types.Log{
			Address: pool1,
			Topics:  []common.Hash{swapEventID},
			Data:    []byte{1, 2, 3, 4}, // This data is too short and will fail to unpack.
		}

		logs := []types.Log{malformedLog}

		// --- Action ---
		_, _, _, _, err := ExtractSwapsFromLogs(logs)

		// --- Assertions ---
		assert.Error(t, err, "Should return an error for malformed logger data")
		assert.Contains(t, err.Error(), "failed to unpack Swap event logger")
	})
}

func TestExtractMintsAndBurnsFromLogs(t *testing.T) {
	// createMintBurnLog is a helper to construct a Mint or Burn logger for testing.
	createMintBurnLog := func(t *testing.T, eventName string, poolAddress common.Address) types.Log {
		event, ok := aerodromeabi.AerodromeABI.Events[eventName]
		require.True(t, ok, "%s event not found in ABI", eventName)

		return types.Log{
			Address: poolAddress,
			Topics:  []common.Hash{event.ID},
			Data:    []byte{}, // Data field is not needed for this test as we only check the topic and address.
		}
	}
	t.Run("should extract and de-duplicate pools from mint and burn logs", func(t *testing.T) {
		pool1 := common.HexToAddress("0x1")
		pool2 := common.HexToAddress("0x2")
		pool3 := common.HexToAddress("0x3")

		logs := []types.Log{
			createMintBurnLog(t, "Mint", pool1),
			createMintBurnLog(t, "Burn", pool2),
			createSwapLog(t, common.HexToAddress("0x4"), 1, big.NewInt(1), big.NewInt(1)), // Irrelevant swap logger
			createMintBurnLog(t, "Mint", pool3),
			createMintBurnLog(t, "Burn", pool1), // Duplicate pool address
		}

		pools := ExtractMintsAndBurnsFromLogs(logs)

		require.Len(t, pools, 3, "Should find 3 unique pools with mint/burn events")
		assert.Contains(t, pools, pool1)
		assert.Contains(t, pools, pool2)
		assert.Contains(t, pools, pool3)
	})

	t.Run("should return an empty slice for logs with no mint or burn events", func(t *testing.T) {
		logs := []types.Log{
			createSwapLog(t, common.HexToAddress("0x4"), 1, big.NewInt(1), big.NewInt(1)),
			{Address: common.HexToAddress("0x3"), Topics: []common.Hash{common.HexToHash("0x1234")}},
		}

		pools := ExtractMintsAndBurnsFromLogs(logs)
		assert.Empty(t, pools, "Should return an empty slice when no mint/burn events are present")
	})

	t.Run("should return an empty slice for an empty logger slice", func(t *testing.T) {
		pools := ExtractMintsAndBurnsFromLogs([]types.Log{})
		assert.Empty(t, pools, "Should handle an empty logger slice gracefully")
	})
}
