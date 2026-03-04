package tickbitmap

import (
	"testing"

	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/stretchr/testify/assert"
)

// makeTickInfoSlice is a helper function to convert a slice of tick indices
// into a slice of TickInfo structs for testing purposes.
func makeTickInfoSlice(indices []int64) []ticks.TickInfo {
	tickInfos := make([]ticks.TickInfo, len(indices))
	for i, idx := range indices {
		tickInfos[i] = ticks.TickInfo{Index: idx}
	}
	return tickInfos
}

func TestNextInitializedTickWithinOneWord(t *testing.T) {
	// A sorted slice of example initialized tick INDICES.
	initializedTickIndices := []int64{-200, -100, -50, 0, 50, 100, 200}

	testCases := []struct {
		name                string
		ticks               []int64 // Using indices for simple test case definitions
		startTick           int64
		lte                 bool // Search direction
		expectedNext        int64
		expectedInitialized bool
	}{
		// --- Search Left (lte = true) ---
		{"LTE: Exact Match", initializedTickIndices, 50, true, 50, true},
		{"LTE: Between Ticks", initializedTickIndices, 40, true, 0, true},
		{"LTE: Just Above a Tick", initializedTickIndices, 51, true, 50, true},
		{"LTE: At First Tick", initializedTickIndices, -200, true, -200, true},
		{"LTE: Before First Tick", initializedTickIndices, -250, true, 0, false},
		{"LTE: At Last Tick", initializedTickIndices, 200, true, 200, true},

		// --- Search Right (lte = false, implemented as >) ---
		{"GT: On an existing tick", initializedTickIndices, 50, false, 100, true},
		{"GT: Between Ticks", initializedTickIndices, 40, false, 50, true},
		{"GT: Just Below a Tick", initializedTickIndices, 49, false, 50, true},
		{"GT: At First Tick", initializedTickIndices, -200, false, -100, true},
		{"GT: At Last Tick", initializedTickIndices, 200, false, 0, false},
		{"GT: After Last Tick", initializedTickIndices, 250, false, 0, false},

		// --- Edge Cases ---
		{"Edge: Empty Slice (LTE)", []int64{}, 100, true, 0, false},
		{"Edge: Empty Slice (GT)", []int64{}, 100, false, 0, false},
		{"Edge: Single Element Match (LTE)", []int64{100}, 100, true, 100, true},
		{"Edge: Single Element No Match (GT)", []int64{100}, 100, false, 0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert the slice of indices to a slice of TickInfo for the function call.
			tickInfoSlice := makeTickInfoSlice(tc.ticks)

			next, initialized := NextInitializedTickWithinOneWord(tickInfoSlice, tc.startTick, tc.lte)

			assert.Equal(t, tc.expectedInitialized, initialized)
			if initialized {
				assert.Equal(t, tc.expectedNext, next)
			}
		})
	}
}
