package ticks

import (
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func TestTickBoundsForPercentWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		currentTick int64
		spacing     uint64
		percent     uint64
		wantLower   int64
		wantUpper   int64
	}{
		{
			name:        "1 percent spacing 1 around zero",
			currentTick: 0,
			spacing:     1,
			percent:     1,
			wantLower:   -101,
			wantUpper:   100,
		},
		{
			name:        "5 percent spacing 1 around zero",
			currentTick: 0,
			spacing:     1,
			percent:     5,
			wantLower:   -513,
			wantUpper:   488,
		},
		{
			name:        "1 percent spacing 10 snaps outward",
			currentTick: 0,
			spacing:     10,
			percent:     1,
			wantLower:   -110,
			wantUpper:   100,
		},
		{
			name:        "5 percent spacing 60 snaps outward",
			currentTick: 0,
			spacing:     60,
			percent:     5,
			wantLower:   -540,
			wantUpper:   540,
		},
		{
			name:        "positive current tick spacing 60",
			currentTick: 123,
			spacing:     60,
			percent:     1,
			wantLower:   0,
			wantUpper:   240,
		},
		{
			name:        "negative current tick spacing 60",
			currentTick: -123,
			spacing:     60,
			percent:     1,
			wantLower:   -240,
			wantUpper:   0,
		},
		{
			name:        "zero percent returns same tick",
			currentTick: 120,
			spacing:     60,
			percent:     0,
			wantLower:   120,
			wantUpper:   120,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotLower, gotUpper := tickBoundsForPercentWindow(tt.currentTick, tt.spacing, tt.percent)
			if gotLower != tt.wantLower || gotUpper != tt.wantUpper {
				t.Fatalf("tickBoundsForPercentWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.currentTick, tt.spacing, tt.percent,
					gotLower, gotUpper,
					tt.wantLower, tt.wantUpper,
				)
			}
		})
	}
}

func TestTickBoundsForPercentWindow_Invariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		currentTick int64
		spacing     uint64
		percent     uint64
	}{
		{0, 1, 1},
		{0, 10, 1},
		{0, 60, 5},
		{12345, 60, 5},
		{-12345, 60, 5},
		{887220, 200, 10},
		{-887220, 200, 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("ct="+itoa64(tt.currentTick), func(t *testing.T) {
			t.Parallel()

			lower, upper := tickBoundsForPercentWindow(tt.currentTick, tt.spacing, tt.percent)

			if lower > upper {
				t.Fatalf("lower > upper: (%d, %d)", lower, upper)
			}

			if tt.spacing > 0 {
				sp := int64(tt.spacing)
				if lower%sp != 0 {
					t.Fatalf("lower %d not aligned to spacing %d", lower, sp)
				}
				if upper%sp != 0 {
					t.Fatalf("upper %d not aligned to spacing %d", upper, sp)
				}
			}

			if lower > tt.currentTick || upper < tt.currentTick {
				t.Fatalf("bounds (%d, %d) do not contain currentTick %d",
					lower, upper, tt.currentTick)
			}
		})
	}
}

func TestSelectTicksForFreshnessWindow(t *testing.T) {
	t.Parallel()

	makeTicks := func(indexes ...int64) []TickInfo {
		out := make([]TickInfo, 0, len(indexes))
		for _, idx := range indexes {
			out = append(out, TickInfo{
				Index:          idx,
				LiquidityGross: big.NewInt(1),
				LiquidityNet:   big.NewInt(1),
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
		return out
	}

	tests := []struct {
		name        string
		currentTick int64
		spacing     uint64
		ticks       []TickInfo
		percent     uint64
		want        []int64
	}{
		{
			name:        "nil ticks",
			currentTick: 0,
			spacing:     60,
			ticks:       nil,
			percent:     5,
			want:        nil,
		},
		{
			name:        "zero percent exact match",
			currentTick: 0,
			spacing:     60,
			ticks:       makeTicks(-60, 0, 60),
			percent:     0,
			want:        []int64{0},
		},
		{
			name:        "zero percent neighbors",
			currentTick: 10,
			spacing:     60,
			ticks:       makeTicks(0, 60, 120),
			percent:     0,
			want:        []int64{0, 60},
		},
		{
			name:        "5 percent window",
			currentTick: 0,
			spacing:     60,
			ticks:       makeTicks(-600, -540, -480, -60, 0, 60, 480, 540, 600),
			percent:     5,
			want:        []int64{-540, -480, -60, 0, 60, 480, 540},
		},
		{
			name:        "1 percent tight window",
			currentTick: 0,
			spacing:     60,
			ticks:       makeTicks(-180, -120, -60, 0, 60, 120, 180),
			percent:     1,
			want:        []int64{-120, -60, 0, 60, 120},
		},
		{
			name:        "sparse ticks",
			currentTick: 0,
			spacing:     1,
			ticks:       makeTicks(-1000, -500, -100, 0, 100, 500),
			percent:     1,
			want:        []int64{-100, 0, 100},
		},
		{
			name:        "no ticks in range",
			currentTick: 0,
			spacing:     1,
			ticks:       makeTicks(-1000, -900, 900, 1000),
			percent:     1,
			want:        nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ticks := cloneAndSortTicks(tt.ticks)
			got := SelectTicksForFreshnessWindow(tt.currentTick, tt.spacing, ticks, tt.percent)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SelectTicksForFreshnessWindow(%d, %d, ticks, %d) = %v, want %v",
					tt.currentTick, tt.spacing, tt.percent, got, tt.want)
			}
		})
	}
}

func TestSelectTicksForFreshnessWindow_BoundsCheck(t *testing.T) {
	t.Parallel()

	ticks := []TickInfo{
		{Index: -600, LiquidityGross: big.NewInt(1)},
		{Index: -540, LiquidityGross: big.NewInt(1)},
		{Index: -480, LiquidityGross: big.NewInt(1)},
		{Index: 0, LiquidityGross: big.NewInt(1)},
		{Index: 480, LiquidityGross: big.NewInt(1)},
		{Index: 540, LiquidityGross: big.NewInt(1)},
	}

	currentTick := int64(0)
	spacing := uint64(60)
	percent := uint64(5)

	lower, upper := tickBoundsForPercentWindow(currentTick, spacing, percent)
	got := SelectTicksForFreshnessWindow(currentTick, spacing, ticks, percent)

	for _, idx := range got {
		if idx < lower || idx > upper {
			t.Fatalf("tick %d outside bounds [%d, %d]", idx, lower, upper)
		}
	}
}

func TestTickBoundsForPercentWindow_Math(t *testing.T) {
	t.Parallel()

	currentTick := int64(0)
	spacing := uint64(1)
	percent := uint64(5)

	gotLower, gotUpper := tickBoundsForPercentWindow(currentTick, spacing, percent)

	p := float64(percent) / 100
	lnBase := math.Log(1.0001)

	wantUpper := int64(math.Ceil(math.Log(1+p) / lnBase))
	wantLower := -int64(math.Ceil(-math.Log(1-p) / lnBase))

	if gotLower != wantLower || gotUpper != wantUpper {
		t.Fatalf("math mismatch: got (%d, %d), want (%d, %d)",
			gotLower, gotUpper, wantLower, wantUpper)
	}
}

func cloneAndSortTicks(in []TickInfo) []TickInfo {
	out := make([]TickInfo, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Index < out[j].Index
	})
	return out
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
