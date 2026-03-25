package bitset

import (
	"testing"
)

func TestBitSet_SetAndIsSet(t *testing.T) {
	// Create a BitSet to hold 100 bits.
	numBits := uint64(100)
	bs := NewBitSet(numBits)

	// Set a few specific bits.
	bs.Set(0)
	bs.Set(63)
	bs.Set(64)
	bs.Set(99)

	// Check that these bits are set.
	if !bs.IsSet(0) {
		t.Error("expected bit 0 to be set")
	}
	if !bs.IsSet(63) {
		t.Error("expected bit 63 to be set")
	}
	if !bs.IsSet(64) {
		t.Error("expected bit 64 to be set")
	}
	if !bs.IsSet(99) {
		t.Error("expected bit 99 to be set")
	}

	// Check that a bit we didn't set is not set.
	if bs.IsSet(1) {
		t.Error("expected bit 1 to be not set")
	}
}

func TestBitSet_Unset(t *testing.T) {
	// Create a BitSet to hold 100 bits.
	numBits := uint64(100)
	bs := NewBitSet(numBits)

	// Set several bits.
	bs.Set(10)
	bs.Set(20)
	bs.Set(30)

	// Confirm they are set.
	if !bs.IsSet(10) || !bs.IsSet(20) || !bs.IsSet(30) {
		t.Error("expected bits 10, 20, and 30 to be set")
	}

	// Now unset bit 20.
	bs.Unset(20)

	// Verify that bit 20 is now cleared, while others remain set.
	if bs.IsSet(20) {
		t.Error("expected bit 20 to be unset")
	}
	if !bs.IsSet(10) || !bs.IsSet(30) {
		t.Error("expected bits 10 and 30 to remain set")
	}
}

func TestBitSet_SetFrom(t *testing.T) {
	// Case 1: Successful copy
	src := BitSet{0b1010, 0b1111}
	dst := BitSet{0, 0}

	dst.SetFrom(src)

	for i := range src {
		if dst[i] != src[i] {
			t.Errorf("BitSet.SetFrom failed: dst[%d]=%b, want %b", i, dst[i], src[i])
		}
	}

	// Case 2: Mismatched size should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("BitSet.SetFrom did not panic on mismatched lengths")
		}
	}()

	shortDst := BitSet{0}
	shortDst.SetFrom(src) // should panic
}
