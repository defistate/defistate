package tickbitmap

import (
	"sort"

	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
)

// NextInitializedTickWithinOneWord finds the next initialized tick in a sorted slice.
// This function is a Go implementation of the logic from Uniswap V3's TickBitmap library,
// adapted to work with a pre-sorted slice of initialized ticks instead of a bitmap.
//
// It uses binary search for efficient lookups.
//
// Parameters:
//   - ticks: A sorted slice of all initialized ticks.
//   - tick: The starting tick for the search.
//   - lte: A boolean indicating the search direction.
//   - If true, it finds the largest initialized tick that is less than or equal to the input `tick`.
//   - If false, it finds the smallest initialized tick that is greater than the input `tick`.
//
// Returns:
//   - next: The next initialized tick found.
//   - initialized: A boolean that is true if an initialized tick was found, and false otherwise.
func NextInitializedTickWithinOneWord(
	ticks []ticks.TickInfo,
	tick int64,
	lte bool,
) (next int64, initialized bool) {
	if len(ticks) == 0 {
		return 0, false
	}

	if lte {
		// --- Search for the next initialized tick to the LEFT (less than or equal to) ---

		// sort.Search performs a binary search to find the smallest index `i`
		// where `ticks[i].Index >= tick`.
		index := sort.Search(len(ticks), func(i int) bool {
			return ticks[i].Index >= tick
		})

		if index < len(ticks) && ticks[index].Index == tick {
			// If the exact tick is found, it's the answer.
			return tick, true
		}

		if index == 0 {
			// If the insertion point is 0, the target tick is smaller than all
			// initialized ticks, so there is no valid tick to the left.
			return 0, false
		}

		// The next initialized tick to the left is at the previous index.
		return ticks[index-1].Index, true

	} else {
		// --- Search for the next initialized tick to the RIGHT (greater than) ---

		// Find the smallest index `i` where `ticks[i].Index > tick`.
		index := sort.Search(len(ticks), func(i int) bool {
			return ticks[i].Index > tick
		})

		if index >= len(ticks) {
			// If the index is out of bounds, the target tick is greater than or equal
			// to all initialized ticks, so there is no valid tick to the right.
			return 0, false
		}

		// The smallest tick greater than the target is at the found index.
		return ticks[index].Index, true
	}
}
