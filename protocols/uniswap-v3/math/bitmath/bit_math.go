package bitmath

import (
	"errors"
	"math/big"
	"math/bits"
)

var (
	// ErrInputIsZero is returned when a function requires a non-zero input but receives zero.
	ErrInputIsZero = errors.New("input must be greater than zero")
	// ErrInputIsNil is returned when a function receives a nil pointer.
	ErrInputIsNil = errors.New("input cannot be nil")
)

// MostSignificantBit returns the index of the most significant bit of the number,
// where the least significant bit is at index 0.
// This function is the Go equivalent of the Solidity BitMath.mostSignificantBit function.
// It uses the highly optimized BitLen() method from the standard library.
//
// The function satisfies the property: x >= 2**msb(x) and x < 2**(msb(x)+1)
func MostSignificantBit(x *big.Int) (uint8, error) {
	if x == nil {
		return 0, ErrInputIsNil
	}
	// x.Sign() returns -1 for < 0, 0 for 0, and +1 for > 0.
	if x.Sign() <= 0 {
		return 0, ErrInputIsZero
	}

	// x.BitLen() returns the number of bits required to represent x.
	// For example, the number 8 (binary 1000) has a bit length of 4.
	// The index of its most significant bit is 3.
	// Therefore, the index is always BitLen() - 1.
	return uint8(x.BitLen() - 1), nil
}

// LeastSignificantBit returns the index of the least significant bit of the number,
// where the least significant bit is at index 0.
// This function is the Go equivalent of the Solidity BitMath.leastSignificantBit function.
//
// The function satisfies the property: (x & 2**lsb(x)) != 0
func LeastSignificantBit(x *big.Int) (uint8, error) {
	if x == nil {
		return 0, ErrInputIsNil
	}
	if x.Sign() <= 0 {
		return 0, ErrInputIsZero
	}

	// The Solidity implementation uses a clever binary search to find the LSB for gas efficiency.
	// In Go, we can achieve this efficiently by checking the words of the big.Int.
	// A big.Int is represented internally as a slice of big.Word.
	words := x.Bits()

	// We iterate through the words to find the first one that is non-zero.
	for i, word := range words {
		if word > 0 {
			// bits.TrailingZeros64 is a highly optimized intrinsic function that counts
			// the number of trailing zero bits in a uint64. This is the index of the LSB
			// within this specific word. We explicitly cast the word to uint64 for safety.
			lsbInWord := bits.TrailingZeros64(uint64(word))
			// The total index is the number of bits in the preceding words (i * 64)
			// plus the index within the current word.
			return uint8(i*64 + lsbInWord), nil
		}
	}

	// This part of the code should be unreachable because we've already checked that x > 0.
	// It's included as a safeguard.
	return 0, ErrInputIsZero
}
