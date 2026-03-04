package ticks

import (
	"fmt"
	"math/big"
	"sort"
)

// Uniswap V3 protocol constants
const (
	MIN_TICK int = -887272
	MAX_TICK int = 887272
)

// GetWordRange calculates the inclusive range of word positions (wordPos) that
// your indexer needs to query for a given tickSpacing.
//
// It returns the starting index (minWord) and the ending index (maxWord).
func getWordRange(tickSpacing int) (minWord int, maxWord int, err error) {
	if tickSpacing <= 0 {
		return 0, 0, fmt.Errorf("tickSpacing must be positive, but got %d", tickSpacing)
	}

	// This calculation gives you the STARTING query index.
	minWord = (MIN_TICK / tickSpacing) >> 8

	// This calculation gives you the ENDING query index.
	maxWord = (MAX_TICK / tickSpacing) >> 8

	return minWord, maxWord, nil
}

// diffBitmaps compares two bitmaps and returns a list of tick indexes that were
// added (became initialized) or removed (became uninitialized).
func diffBitmaps(oldMap, newMap Bitmap, tickSpacing int64) (addedTicks, removedTicks []int64) {
	// Use a map to collect all unique word positions from both bitmaps.
	// This ensures we check words that were added, removed, or modified.
	allWordPositions := make(map[int16]struct{})
	for pos := range oldMap {
		allWordPositions[pos] = struct{}{}
	}
	for pos := range newMap {
		allWordPositions[pos] = struct{}{}
	}

	// For each relevant word position, compare the old and new words.
	for pos := range allWordPositions {
		oldWord, _ := oldMap[pos]
		newWord, _ := newMap[pos]

		// To handle cases where a word is new or was completely removed,
		// treat nil (missing) words as a big.Int of 0.
		if oldWord == nil {
			oldWord = big.NewInt(0)
		}
		if newWord == nil {
			newWord = big.NewInt(0)
		}

		// If words are identical, there's no change, so we can skip.
		if oldWord.Cmp(newWord) == 0 {
			continue
		}

		// Use bitwise XOR (^) to find all bits that have flipped between the two words.
		changedBits := new(big.Int).Xor(oldWord, newWord)

		// Now, iterate through the 256 bits of the 'changedBits' word.
		for i := range 256 {
			// If the bit at position 'i' is 1, it means this tick's state has flipped.
			if changedBits.Bit(i) == 1 {
				// Calculate the actual tick index from the word position and bit position.
				compressedTick := (int64(pos) * 256) + int64(i)
				tickIndex := compressedTick * tickSpacing
				// To determine if it was an ADD or a REMOVE, check the bit in the NEW word.
				if newWord.Bit(i) == 1 {
					// The bit is 1 in the new map, so it must have been 0 before. This is an addition.
					addedTicks = append(addedTicks, tickIndex)
				} else {
					// The bit is 0 in the new map, so it must have been 1 before. This is a removal.
					removedTicks = append(removedTicks, tickIndex)
				}
			}
		}
	}

	return addedTicks, removedTicks
}

// TicksFromBitmap translates a tick bitmap into a sorted slice of all
// initialized tick indexes.
func TicksFromBitmap(bitmap Bitmap, tickSpacing int64) []int64 {
	// Pre-allocate a slice with a reasonable starting capacity if the map is not empty.
	// This is a small optimization to reduce re-allocations.
	var ticksToRequest []int64
	if len(bitmap) > 0 {
		// A crude but effective way to estimate capacity. Assumes partial sparsity.
		ticksToRequest = make([]int64, 0, len(bitmap)*16)
	}

	// Iterate through each word in the bitmap.
	for wordPos, word := range bitmap {
		// If a word is nil or zero, there's nothing to do for this range of 256 ticks.
		if word == nil || word.Sign() == 0 {
			continue
		}

		// Check each of the 256 bits in the word.
		for bit := 0; bit < 256; bit++ {
			// If the bit is set to 1, it represents an initialized tick.
			if word.Bit(bit) == 1 {
				// 1. Reconstruct the compressed tick index from its word and bit position.
				compressedIndex := (int64(wordPos) * 256) + int64(bit)

				// 2. Uncompress the index to get the actual tick by multiplying by the spacing.
				realTickIndex := compressedIndex * tickSpacing

				ticksToRequest = append(ticksToRequest, realTickIndex)
			}
		}
	}

	// Although the iteration order isn't guaranteed, sorting the final list
	// provides a deterministic output, which is good practice.
	sort.Slice(ticksToRequest, func(i, j int) bool {
		return ticksToRequest[i] < ticksToRequest[j]
	})

	return ticksToRequest
}
