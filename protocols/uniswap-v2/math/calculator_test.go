package uniswapv2

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBigIntFromString is a helper function to create a big.Int from a string,
// which is necessary for numbers larger than a standard int64.
func newBigIntFromString(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("failed to set string for big.Int")
	}
	return n
}

func TestGetAmountOut(t *testing.T) {
	// --- Test Cases Setup ---
	testCases := []struct {
		name           string
		amountIn       *big.Int
		tokenIn        uint64
		tokenOut       uint64
		pool           uniswapv2.PoolView
		expectedAmount *big.Int
		expectError    bool
		expectedErr    error // Use specific error types for checking
	}{
		{
			name:     "Standard Swap (Token0 -> Token1)",
			amountIn: big.NewInt(1_000_000), // 1 USDC (6 decimals)
			tokenIn:  0,
			tokenOut: 1,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,                                           // USDC
				Token1:   1,                                           // WETH
				Reserve0: big.NewInt(100_000_000),                     // 100 USDC
				Reserve1: newBigIntFromString("50000000000000000000"), // 50 WETH (18 decimals)
				FeeBps:   30,
			},
			expectedAmount: newBigIntFromString("493579017198530649"),
			expectError:    false,
		},
		{
			name:     "Standard Swap (Token1 -> Token0)",
			amountIn: newBigIntFromString("1000000000000000000"), // 1 WETH
			tokenIn:  1,
			tokenOut: 0,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
				FeeBps:   30,
			},
			expectedAmount: big.NewInt(1955016),
			expectError:    false,
		},
		{
			name:     "Swap with Different Fee",
			amountIn: big.NewInt(1_000_000),
			tokenIn:  0,
			tokenOut: 1,
			pool: uniswapv2.PoolView{
				ID:       2,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
				FeeBps:   100, // 1% fee
			},
			expectedAmount: newBigIntFromString("490147539360332706"),
			expectError:    false,
		},
		{
			name:     "Edge Case: Zero Liquidity",
			amountIn: big.NewInt(1_000_000),
			tokenIn:  0,
			tokenOut: 1,
			pool: uniswapv2.PoolView{
				ID:       3,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(0), // Zero reserve
				Reserve1: newBigIntFromString("50000000000000000000"),
				FeeBps:   30,
			},
			expectedAmount: big.NewInt(0),
			expectError:    false,
		},
		{
			name:        "Invalid Input: Nil AmountIn",
			amountIn:    nil,
			tokenIn:     0,
			tokenOut:    1,
			pool:        uniswapv2.PoolView{},
			expectError: true,
			expectedErr: ErrNilAmount,
		},
		{
			name:        "Invalid Input: Negative AmountIn",
			amountIn:    big.NewInt(-100),
			tokenIn:     0,
			tokenOut:    1,
			pool:        uniswapv2.PoolView{},
			expectError: true,
			expectedErr: ErrInvalidAmount,
		},
		{
			name:     "Invalid Input: Token Mismatch",
			amountIn: big.NewInt(1_000_000),
			tokenIn:  99, // This token is not in the pool
			tokenOut: 1,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
			},
			expectError: true,
			expectedErr: ErrTokenMismatch,
		},
	}

	// --- Run Test Cases ---
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the standalone, pooled GetAmountOut function.
			amountOut, err := GetAmountOut(tc.amountIn, tc.tokenIn, tc.tokenOut, tc.pool)

			if tc.expectError {
				require.Error(t, err)
				// Use errors.Is for robust, type-safe error checking.
				assert.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, amountOut)
				// Use Cmp for reliable big.Int comparison
				assert.Zero(t, tc.expectedAmount.Cmp(amountOut), "Expected %s, but got %s", tc.expectedAmount.String(), amountOut.String())
			}
		})
	}
}

func TestGetAmountIn(t *testing.T) {
	testCases := []struct {
		name           string
		amountOut      *big.Int
		tokenIn        uint64
		tokenOut       uint64
		pool           uniswapv2.PoolView
		expectedAmount *big.Int
		expectError    bool
		expectedErr    error
	}{
		{
			name:      "Standard Swap (Token0 -> Token1)",
			amountOut: newBigIntFromString("493579017198530649"),
			tokenIn:   0,
			tokenOut:  1,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
				FeeBps:   30,
			},
			expectedAmount: big.NewInt(1000000), // Corrected expected value
			expectError:    false,
		},
		{
			name:      "Standard Swap (Token1 -> Token0)",
			amountOut: big.NewInt(1955016),
			tokenIn:   1,
			tokenOut:  0,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
				FeeBps:   30,
			},
			expectedAmount: newBigIntFromString("999999498234537320"), // Corrected expected value
			expectError:    false,
		},
		{
			name:        "Invalid Input: Nil AmountOut",
			amountOut:   nil,
			expectError: true,
			expectedErr: ErrNilAmount,
		},
		{
			name:        "Invalid Input: Negative AmountOut",
			amountOut:   big.NewInt(-100),
			expectError: true,
			expectedErr: ErrInvalidAmount,
		},
		{
			name:      "Invalid State: Insufficient Liquidity",
			amountOut: newBigIntFromString("60000000000000000000"), // Request more than is in the pool
			tokenIn:   0,
			tokenOut:  1,
			pool: uniswapv2.PoolView{
				ID:       1,
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(100_000_000),
				Reserve1: newBigIntFromString("50000000000000000000"),
			},
			expectError: true,
			expectedErr: ErrInsufficientLiquidity,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			amountIn, err := GetAmountIn(tc.amountOut, tc.tokenIn, tc.tokenOut, tc.pool)

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, amountIn)
				assert.Zero(t, tc.expectedAmount.Cmp(amountIn), "Expected %s, but got %s", tc.expectedAmount.String(), amountIn.String())
			}
		})
	}
}

func TestSimulateSwap(t *testing.T) {
	pool := uniswapv2.PoolView{
		ID:       1,
		Token0:   0,
		Token1:   1,
		Reserve0: big.NewInt(100_000_000),
		Reserve1: newBigIntFromString("50000000000000000000"),
		FeeBps:   30,
	}
	amountIn := big.NewInt(1_000_000)

	amountOut, newPool, err := SimulateSwap(amountIn, 0, 1, pool)
	require.NoError(t, err)

	// Check amountOut
	expectedAmountOut := newBigIntFromString("493579017198530649")
	assert.Zero(t, expectedAmountOut.Cmp(amountOut))

	// Check new reserves
	expectedReserve0 := new(big.Int).Add(pool.Reserve0, amountIn)
	expectedReserve1 := new(big.Int).Sub(pool.Reserve1, amountOut)
	assert.Zero(t, expectedReserve0.Cmp(newPool.Reserve0))
	assert.Zero(t, expectedReserve1.Cmp(newPool.Reserve1))
}

// TestSimulateSwap_IdempotencyAndStateIsolation verifies that the simulation
// function does not mutate its inputs and that the returned new state is a
// proper deep copy of its mutable fields, preventing side effects.
func TestSimulateSwap_IdempotencyAndStateIsolation(t *testing.T) {
	// 1. Arrange: Create the initial, pristine pool state.
	originalPool := uniswapv2.PoolView{
		ID:       1,
		Token0:   0,
		Token1:   1,
		Reserve0: big.NewInt(100_000_000),
		Reserve1: newBigIntFromString("50000000000000000000"),
		FeeBps:   30,
	}
	amountIn := big.NewInt(1_000_000)
	tokenInID := uint64(0)
	tokenOutID := uint64(1)

	// 2. Act: Run the simulation twice on the *same original state*.
	amountOut1, newPoolState1, err1 := SimulateSwap(amountIn, tokenInID, tokenOutID, originalPool)
	require.NoError(t, err1, "First simulation should succeed")

	amountOut2, newPoolState2, err2 := SimulateSwap(amountIn, tokenInID, tokenOutID, originalPool)
	require.NoError(t, err2, "Second simulation should succeed")

	// 3. Assert: Verify idempotency and state isolation.
	t.Run("Idempotency Check", func(t *testing.T) {
		// This proves that the first simulation did not mutate the 'originalPool' object.
		// If it had, the second simulation would have started from a different state
		// and produced a different result.
		assert.Equal(t, amountOut1.String(), amountOut2.String(), "Amount out should be identical on consecutive runs")
		assert.True(t, reflect.DeepEqual(newPoolState1, newPoolState2), "The new pool state should be identical on consecutive runs")
	})

	t.Run("Deep Copy Check (Reserves)", func(t *testing.T) {
		// This proves that the mutable *big.Int fields in the new state are new
		// instances in memory, not just copies of the original pointers.
		assert.NotSame(t, originalPool.Reserve0, newPoolState1.Reserve0, "New state's Reserve0 should be a new big.Int instance")
		assert.NotSame(t, originalPool.Reserve1, newPoolState1.Reserve1, "New state's Reserve1 should be a new big.Int instance")
	})

	t.Run("Result Isolation Check", func(t *testing.T) {
		// This is the definitive test. We modify the result of the first simulation
		// and verify that the result of the second simulation is not affected.
		// This proves that the two returned states are truly independent of each other.
		originalReserve2 := new(big.Int).Set(newPoolState2.Reserve0)

		// Mutate the result of the first simulation
		newPoolState1.Reserve0.Add(newPoolState1.Reserve0, big.NewInt(12345))

		// Assert that the second result remains unchanged
		assert.NotEqual(t, newPoolState1.Reserve0.String(), newPoolState2.Reserve0.String(), "Modifying state 1 should not affect state 2")
		assert.Equal(t, originalReserve2.String(), newPoolState2.Reserve0.String(), "State 2's Reserve0 should remain pristine")
	})
}

// --- Benchmarks ---

// result is a package-level variable to ensure the compiler does not optimize away the benchmarked function call.
var result *big.Int
var resultPool uniswapv2.PoolView

func BenchmarkGetAmountOut(b *testing.B) {
	// Setup a realistic pool and inputs for the benchmark.
	pool := uniswapv2.PoolView{
		ID:       1,
		Token0:   0,
		Token1:   1,
		Reserve0: newBigIntFromString("2000000000000"),          // 2,000,000 USDC
		Reserve1: newBigIntFromString("1000000000000000000000"), // 1,000 WETH
		FeeBps:   30,
	}
	amountIn := newBigIntFromString("1000000000000000000") // 1 WETH
	tokenIn := uint64(1)
	tokenOut := uint64(0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		amountOut, _ := GetAmountOut(amountIn, tokenIn, tokenOut, pool)
		result = amountOut
	}
}

func BenchmarkGetAmountIn(b *testing.B) {
	pool := uniswapv2.PoolView{
		ID:       1,
		Token0:   0,
		Token1:   1,
		Reserve0: newBigIntFromString("2000000000000"),
		Reserve1: newBigIntFromString("1000000000000000000000"),
		FeeBps:   30,
	}
	amountOut := newBigIntFromString("1994000000") // ~1994 USDC
	tokenIn := uint64(1)
	tokenOut := uint64(0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		amountIn, _ := GetAmountIn(amountOut, tokenIn, tokenOut, pool)
		result = amountIn
	}
}

func BenchmarkSimulateSwap(b *testing.B) {
	pool := uniswapv2.PoolView{
		ID:       1,
		Token0:   0,
		Token1:   1,
		Reserve0: newBigIntFromString("2000000000000"),
		Reserve1: newBigIntFromString("1000000000000000000000"),
		FeeBps:   30,
	}
	amountIn := newBigIntFromString("1000000000000000000")
	tokenIn := uint64(1)
	tokenOut := uint64(0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		amountOut, newPool, _ := SimulateSwap(amountIn, tokenIn, tokenOut, pool)
		result = amountOut
		resultPool = newPool
	}
}

// TestGetExchangeRate provides a suite of tests for the V2 GetexchangeRate function.
func TestGetExchangeRate(t *testing.T) {
	// Mock Pool: Assume Token 0 is WETH (18 decimals) and Token 1 is USDC (6 decimals)
	// Price: 3,000 USDC per WETH
	reserve0 := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))   // 1,000 WETH
	reserve1 := new(big.Int).Mul(big.NewInt(3000000), new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)) // 3,000,000 USDC

	mockPool := uniswapv2.PoolView{
		Token0:   0, // WETH
		Token1:   1, // USDC
		Reserve0: reserve0,
		Reserve1: reserve1,
	}

	// Define test cases
	testCases := []struct {
		name          string
		tokenInID     uint64
		tokenOutID    uint64
		decimalsIn    uint8
		decimalsOut   uint8
		pool          uniswapv2.PoolView
		expectedPrice string
		expectError   bool
	}{
		{
			name:          "Native Direction: WETH (18) -> USDC (6)",
			tokenInID:     0,
			tokenOutID:    1,
			decimalsIn:    18,
			decimalsOut:   6,
			pool:          mockPool,
			expectedPrice: "2970297029", // Represents 2970 USDC (scaled by 6 decimals)
			expectError:   false,
		},
		{
			name:          "Inverse Direction: USDC (6) -> WETH (18)",
			tokenInID:     1,
			tokenOutID:    0,
			decimalsIn:    6,
			decimalsOut:   18,
			pool:          mockPool,
			expectedPrice: "330033003300330", // Represents ~0.00033 WETH (scaled by 18 decimals)
			expectError:   false,
		},
		{
			name:          "Mismatched Tokens: Should return an error",
			tokenInID:     2, // A token not in the pool
			tokenOutID:    0,
			decimalsIn:    18,
			decimalsOut:   18,
			pool:          mockPool,
			expectedPrice: "0",
			expectError:   true,
		},
		{
			name:        "Edge Case: Zero Reserve in Denominator",
			tokenInID:   0,
			tokenOutID:  1,
			decimalsIn:  18,
			decimalsOut: 6,
			pool: uniswapv2.PoolView{ // Pool with a zero reserve
				Token0:   0,
				Token1:   1,
				Reserve0: big.NewInt(0),
				Reserve1: reserve1,
			},
			expectedPrice: "0",
			expectError:   true, // Expecting a "division by zero" error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function under test
			exchangeRate, err := GetExchangeRate(tc.tokenInID, tc.tokenOutID, tc.decimalsIn, tc.decimalsOut, tc.pool)

			// Check for expected error
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				return
			}

			// Check if an unexpected error occurred
			if err != nil {
				t.Fatalf("Expected no error, but got: %v", err)
			}

			// Convert expected string to big.Int for comparison
			expectedBigInt, ok := new(big.Int).SetString(tc.expectedPrice, 10)
			if !ok {
				t.Fatalf("Invalid expectedPrice string in test case: %s", tc.expectedPrice)
			}

			// Compare the actual result with the expected result
			if exchangeRate.Cmp(expectedBigInt) != 0 {
				t.Errorf("Mismatch in spot price.\nExpected: %s\nGot:      %s", expectedBigInt.String(), exchangeRate.String())
			}

			fmt.Printf("✅ Test '%s' passed. Result: %s\n", tc.name, exchangeRate.String())
		})
	}
}
