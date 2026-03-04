package forks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetForkData(t *testing.T) {
	testCases := []struct {
		name    string
		forkID  uint8
		wantErr bool
	}{
		{
			name:    "UniswapV3",
			forkID:  UniswapV3,
			wantErr: false,
		},
		{
			name:    "PancakeswapV3",
			forkID:  PancakeswapV3,
			wantErr: false,
		},
		{
			name:    "Aerodrome",
			forkID:  Aerodrome,
			wantErr: false,
		},
		{
			name:    "OmniExchange",
			forkID:  OmniExchange,
			wantErr: false,
		},
		{
			name:    "Unsupported Fork",
			forkID:  99, // An ID not in the switch statement
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			forkData, err := GetForkData(tc.forkID)

			if tc.wantErr {
				require.Error(t, err, "Expected an error for an unsupported fork ID")
				return
			}

			require.NoError(t, err, "Did not expect an error for a supported fork ID")

			// --- Validate SystemFuncs ---
			// Ensure all function pointers required by the main system are non-nil.
			sys := forkData.System
			assert.NotNil(t, sys.PoolInitializer, "System.PoolInitializer should be set")
			assert.NotNil(t, sys.PoolInfoProvider, "System.PoolInfoProvider should be set")
			assert.NotNil(t, sys.DiscoverPools, "System.DiscoverPools should be set")
			assert.NotNil(t, sys.ExtractSwaps, "System.ExtractSwaps should be set")
			assert.NotNil(t, sys.ExtractMintsAndBurns, "System.ExtractMintsAndBurns should be set")
			assert.NotNil(t, sys.TestBloom, "System.TestBloom should be set")
			assert.NotEmpty(t, sys.FilterTopics, "System.FilterTopics should not be empty")

			// --- Validate TickIndexerFuncs ---
			// Ensure all function pointers required by the tick indexer are non-nil.
			ti := forkData.TickIndexer
			assert.NotNil(t, ti.TickBitmapProvider, "TickIndexer.TickBitmapProvider should be set")
			assert.NotNil(t, ti.TickInfoProvider, "TickIndexer.TickInfoProvider should be set")
			assert.NotNil(t, ti.UpdatedInBlock, "TickIndexer.UpdatedInBlock should be set")
			assert.NotNil(t, ti.TestBloom, "TickIndexer.TestBloom should be set")
			assert.NotEmpty(t, ti.FilterTopics, "TickIndexer.FilterTopics should not be empty")
		})
	}
}
