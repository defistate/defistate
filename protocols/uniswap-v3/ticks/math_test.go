package ticks

import (
	"math/big"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// createTestBitmapFromTicks is a test helper to easily generate a Bitmap
// from a simple list of initialized tick indexes. This makes test
// cases much more readable.
func createTestBitmapFromTicks(ticks ...int64) Bitmap {
	if len(ticks) == 0 {
		return make(Bitmap)
	}
	bitmap := make(Bitmap)
	for _, tick := range ticks {
		wordPos := int16(tick / 256)
		bitPos := int(tick % 256)

		if _, ok := bitmap[wordPos]; !ok {
			bitmap[wordPos] = big.NewInt(0)
		}
		bitmap[wordPos].SetBit(bitmap[wordPos], bitPos, 1)
	}
	return bitmap
}

func TestDiffBitmaps(t *testing.T) {
	// Helper function to create a compressed bitmap from full tick indexes for testing.
	// This makes test cases much more readable and ensures valid inputs.
	createCompressedBitmap := func(spacing int64, ticks ...int64) Bitmap {
		if len(ticks) == 0 {
			return createTestBitmapFromTicks()
		}
		compressedTicks := make([]int64, len(ticks))
		for i, tick := range ticks {
			// Pre-validate that the test data is correct.
			if spacing > 0 && tick%spacing != 0 {
				t.Fatalf("invalid test data: tick %d is not a multiple of spacing %d", tick, spacing)
			}
			compressedTicks[i] = tick / spacing
		}
		return createTestBitmapFromTicks(compressedTicks...)
	}

	// Define the test table
	testCases := []struct {
		name        string
		tickSpacing int64
		oldTicks    []int64
		newTicks    []int64
		wantAdded   []int64
		wantRemoved []int64
	}{
		{
			name:        "spacing 60: no changes",
			tickSpacing: 60,
			oldTicks:    []int64{120, 240},
			newTicks:    []int64{120, 240},
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name:        "spacing 10: add one tick",
			tickSpacing: 10,
			oldTicks:    []int64{100},
			newTicks:    []int64{100, 200},
			wantAdded:   []int64{200},
			wantRemoved: nil,
		},
		{
			name:        "spacing 10: remove one tick",
			tickSpacing: 10,
			oldTicks:    []int64{100, 200},
			newTicks:    []int64{100},
			wantAdded:   nil,
			wantRemoved: []int64{200},
		},
		{
			name:        "spacing 60: modify tick within same word (add and remove)",
			tickSpacing: 60,
			oldTicks:    []int64{120, 240},
			newTicks:    []int64{120, 300},
			wantAdded:   []int64{300},
			wantRemoved: []int64{240},
		},
		{
			name:        "spacing 100: add a new word",
			tickSpacing: 100,
			oldTicks:    []int64{100},        // Compressed word 0
			newTicks:    []int64{100, 25600}, // Compressed word 1
			wantAdded:   []int64{25600},
			wantRemoved: nil,
		},
		{
			name:        "spacing 100: remove an entire word",
			tickSpacing: 100,
			oldTicks:    []int64{100, 25600},
			newTicks:    []int64{100},
			wantAdded:   nil,
			wantRemoved: []int64{25600},
		},
		{
			name:        "spacing 1: complex changes (no compression)",
			tickSpacing: 1,
			oldTicks:    []int64{1, 5000, 5001},
			newTicks:    []int64{2, 5000, 6000},
			wantAdded:   []int64{2, 6000},
			wantRemoved: []int64{1, 5001},
		},
		{
			name:        "spacing 200: complex changes with large spacing",
			tickSpacing: 200,
			oldTicks:    []int64{200, 100000, 100200}, // compressed: 1, 500, 501
			newTicks:    []int64{400, 100000, 120000}, // compressed: 2, 500, 600
			wantAdded:   []int64{400, 120000},
			wantRemoved: []int64{200, 100200},
		},
		{
			name:        "spacing 60: boundary case with tick 0",
			tickSpacing: 60,
			oldTicks:    []int64{},
			newTicks:    []int64{0, 60},
			wantAdded:   []int64{0, 60},
			wantRemoved: nil,
		},
	}

	// Run the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			// Create the compressed bitmaps using the helper
			oldMap := createCompressedBitmap(tc.tickSpacing, tc.oldTicks...)
			newMap := createCompressedBitmap(tc.tickSpacing, tc.newTicks...)

			// Call the function under test with the correct spacing
			gotAdded, gotRemoved := diffBitmaps(oldMap, newMap, tc.tickSpacing)

			// Sort all slices for deterministic comparison
			sort.Slice(gotAdded, func(i, j int) bool { return gotAdded[i] < gotAdded[j] })
			sort.Slice(gotRemoved, func(i, j int) bool { return gotRemoved[i] < gotRemoved[j] })
			sort.Slice(tc.wantAdded, func(i, j int) bool { return tc.wantAdded[i] < tc.wantAdded[j] })
			sort.Slice(tc.wantRemoved, func(i, j int) bool { return tc.wantRemoved[i] < tc.wantRemoved[j] })

			// Assert equality
			assert.Equal(tc.wantAdded, gotAdded, "should have the correct set of added ticks")
			assert.Equal(tc.wantRemoved, gotRemoved, "should have the correct set of removed ticks")
		})
	}
}
