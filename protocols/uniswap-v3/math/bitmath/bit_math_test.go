package bitmath

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMostSignificantBit(t *testing.T) {
	testCases := []struct {
		name     string
		input    *big.Int
		expected uint8
		err      error
	}{
		{"Input 1", big.NewInt(1), 0, nil},
		{"Input 2", big.NewInt(2), 1, nil},
		{"Input 3", big.NewInt(3), 1, nil},
		{"Input 255", big.NewInt(255), 7, nil},
		{"Input 256", big.NewInt(256), 8, nil},
		{"Large Number (2^128 - 1)", new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1)), 127, nil},
		{"Large Number (2^128)", new(big.Int).Lsh(big.NewInt(1), 128), 128, nil},
		{"Error on Zero", big.NewInt(0), 0, ErrInputIsZero},
		{"Error on Nil", nil, 0, ErrInputIsNil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := MostSignificantBit(tc.input)
			if tc.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestLeastSignificantBit(t *testing.T) {
	testCases := []struct {
		name     string
		input    *big.Int
		expected uint8
		err      error
	}{
		{"Input 1", big.NewInt(1), 0, nil},
		{"Input 2", big.NewInt(2), 1, nil},
		{"Input 3", big.NewInt(3), 0, nil},   // binary 11, LSB is at index 0
		{"Input 8", big.NewInt(8), 3, nil},   // binary 1000
		{"Input 10", big.NewInt(10), 1, nil}, // binary 1010
		{"Large Number (2^128)", new(big.Int).Lsh(big.NewInt(1), 128), 128, nil},
		{"Large Number (2^128 + 2^64)", new(big.Int).Or(new(big.Int).Lsh(big.NewInt(1), 128), new(big.Int).Lsh(big.NewInt(1), 64)), 64, nil},
		{"Error on Zero", big.NewInt(0), 0, ErrInputIsZero},
		{"Error on Nil", nil, 0, ErrInputIsNil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := LeastSignificantBit(tc.input)
			if tc.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// --- Invariant Tests (Fuzzing) ---

func TestMostSignificantBit_Invariant(t *testing.T) {
	// This test simulates fuzzing by running on a large number of random inputs
	// to verify the mathematical properties of the function.
	for i := 0; i < 1000; i++ {
		// Generate a random 256-bit integer > 0
		input, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 256))
		require.NoError(t, err)
		if input.Sign() == 0 {
			input.SetInt64(1) // Ensure input > 0
		}

		msb, err := MostSignificantBit(input)
		require.NoError(t, err)

		// Invariant 1: input >= 2**msb
		lowerBound := new(big.Int).Lsh(big.NewInt(1), uint(msb))
		assert.True(t, input.Cmp(lowerBound) >= 0, "Invariant failed: input %s should be >= 2**%d (%s)", input, msb, lowerBound)

		// Invariant 2: msb == 255 || input < 2**(msb + 1)
		if msb < 255 {
			upperBound := new(big.Int).Lsh(big.NewInt(1), uint(msb+1))
			assert.True(t, input.Cmp(upperBound) < 0, "Invariant failed: input %s should be < 2**%d (%s)", input, msb+1, upperBound)
		}
	}
}

func TestLeastSignificantBit_Invariant(t *testing.T) {
	// This test simulates fuzzing to verify mathematical properties.
	for i := 0; i < 1000; i++ {
		// Generate a random 256-bit integer > 0
		input, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 256))
		require.NoError(t, err)
		if input.Sign() == 0 {
			input.SetInt64(1) // Ensure input > 0
		}

		lsb, err := LeastSignificantBit(input)
		require.NoError(t, err)

		// Invariant 1: (input & 2**lsb) != 0
		powerOfTwo := new(big.Int).Lsh(big.NewInt(1), uint(lsb))
		result := new(big.Int).And(input, powerOfTwo)
		assert.NotZero(t, result.Sign(), "Invariant failed: (input %s & 2**%d) should not be zero", input, lsb)

		// Invariant 2: (input & (2**lsb - 1)) == 0
		mask := new(big.Int).Sub(powerOfTwo, big.NewInt(1))
		result2 := new(big.Int).And(input, mask)
		assert.Zero(t, result2.Sign(), "Invariant failed: (input %s & (2**%d - 1)) should be zero", input, lsb)
	}
}
