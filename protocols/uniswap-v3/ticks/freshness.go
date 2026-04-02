package ticks

import (
	"math"
	"sort"
)

// SelectTicksForFreshnessWindow returns the initialized tick indexes that fall within
// the configured ± percentage freshness band around currentTick.
//
// Assumptions:
// - ticks is sorted ascending by Index
// - tickFreshnessGuaranteePercent is a whole-number percent, e.g. 5 means ±5%
// - currentTick is the pool's current active tick
//
// Notes:
//   - Tick spacing determines valid initialized tick boundaries, so the computed lower/upper
//     bounds are snapped outward to spacing multiples.
//   - This function returns tick indexes to fetch, not TickInfo objects, since that is what
//     the on-chain fetch path usually wants.
func SelectTicksForFreshnessWindow(
	currentTick int64,
	tickSpacing uint64,
	ticks []TickInfo,
	tickFreshnessGuaranteePercent uint64,
) []int64 {
	if len(ticks) == 0 {
		return nil
	}
	if tickSpacing == 0 {
		return nil
	}
	if tickFreshnessGuaranteePercent == 0 {
		// Return the nearest valid initialized tick(s) around currentTick.
		// Since the caller asked for 0%, we interpret this as "only the current active neighborhood".
		idx := sort.Search(len(ticks), func(i int) bool {
			return ticks[i].Index >= currentTick
		})

		switch {
		case idx == 0:
			return []int64{ticks[0].Index}
		case idx == len(ticks):
			return []int64{ticks[len(ticks)-1].Index}
		default:
			// Return both neighbors to avoid ambiguity around the active boundary.
			if ticks[idx].Index == currentTick {
				return []int64{ticks[idx].Index}
			}
			return []int64{ticks[idx-1].Index, ticks[idx].Index}
		}
	}

	lowerTick, upperTick := tickBoundsForPercentWindow(currentTick, tickSpacing, tickFreshnessGuaranteePercent)

	left := sort.Search(len(ticks), func(i int) bool {
		return ticks[i].Index >= lowerTick
	})
	right := sort.Search(len(ticks), func(i int) bool {
		return ticks[i].Index > upperTick
	})

	if left >= right {
		return nil
	}

	out := make([]int64, 0, right-left)
	for i := left; i < right; i++ {
		out = append(out, ticks[i].Index)
	}
	return out
}

// tickBoundsForPercentWindow converts a ±percent price band around currentTick
// into inclusive lower/upper tick bounds, snapped outward to valid tickSpacing multiples.
func tickBoundsForPercentWindow(
	currentTick int64,
	tickSpacing uint64,
	percent uint64,
) (int64, int64) {
	p := float64(percent) / 100.0

	// Exact conversion:
	// upperDelta = ln(1 + p) / ln(1.0001)
	// lowerDelta = ln(1 - p) / ln(1.0001)
	//
	// For the lower side, we use the magnitude of the negative move.
	lnBase := math.Log(1.0001)

	upperDelta := int64(math.Ceil(math.Log(1.0+p) / lnBase))

	var lowerDelta int64
	if p >= 1.0 {
		// 100% downward move is not meaningful here; clamp to a very large band.
		lowerDelta = upperDelta
	} else {
		lowerDelta = int64(math.Ceil(-math.Log(1.0-p) / lnBase))
	}

	lower := currentTick - lowerDelta
	upper := currentTick + upperDelta

	spacing := int64(tickSpacing)

	// Snap outward so we don't accidentally exclude valid spacing-aligned ticks.
	lower = floorToSpacing(lower, spacing)
	upper = ceilToSpacing(upper, spacing)

	return lower, upper
}

func floorToSpacing(v, spacing int64) int64 {
	if spacing <= 0 {
		return v
	}
	rem := v % spacing
	if rem == 0 {
		return v
	}
	if v >= 0 {
		return v - rem
	}
	return v - rem - spacing
}

func ceilToSpacing(v, spacing int64) int64 {
	if spacing <= 0 {
		return v
	}
	rem := v % spacing
	if rem == 0 {
		return v
	}
	if v >= 0 {
		return v - rem + spacing
	}
	return v - rem
}
