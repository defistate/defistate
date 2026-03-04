package abi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseERC20TransferEvent uses a table-driven approach to test various scenarios
// for the ParseERC20TransferEvent function
func TestParseERC20TransferEvent(t *testing.T) {
	// --- Test Fixtures ---
	fromAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	toAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	oneToken := new(big.Int).SetInt64(1000000000000000000)
	zeroToken := new(big.Int).SetInt64(0)

	// Pre-calculate topics and data for reusability
	transferEventID := ERC20ABI.Events["Transfer"].ID
	fromTopic := common.HexToHash(fromAddr.Hex())
	toTopic := common.HexToHash(toAddr.Hex())
	oneTokenData := common.LeftPadBytes(oneToken.Bytes(), 32)
	zeroTokenData := common.LeftPadBytes(zeroToken.Bytes(), 32)
	malformedData := []byte{1, 2, 3, 4} // Not a 32-byte value

	approvalEventID := crypto.Keccak256Hash([]byte("Approval(address,address,uint256)"))

	// --- Test Cases ---
	testCases := []struct {
		name          string
		logger        types.Log
		expectErr     bool
		expectedFrom  common.Address
		expectedTo    common.Address
		expectedValue *big.Int
		errContains   string
	}{
		{
			name: "Happy Path - Valid Transfer",
			logger: types.Log{
				Topics: []common.Hash{transferEventID, fromTopic, toTopic},
				Data:   oneTokenData,
			},
			expectErr:     false,
			expectedFrom:  fromAddr,
			expectedTo:    toAddr,
			expectedValue: oneToken,
		},
		{
			name: "Edge Case - Zero Value Transfer",
			logger: types.Log{
				Topics: []common.Hash{transferEventID, fromTopic, toTopic},
				Data:   zeroTokenData,
			},
			expectErr:     false,
			expectedFrom:  fromAddr,
			expectedTo:    toAddr,
			expectedValue: zeroToken,
		},
		{
			name: "Error - Incorrect Event ID",
			logger: types.Log{
				Topics: []common.Hash{approvalEventID, fromTopic, toTopic}, // Wrong event ID
				Data:   oneTokenData,
			},
			expectErr:   true,
			errContains: "logger is not a valid ERC20 Transfer event",
		},
		{
			name: "Error - Too Few Topics",
			logger: types.Log{
				Topics: []common.Hash{transferEventID, fromTopic}, // Missing 'to' topic
				Data:   oneTokenData,
			},
			expectErr:   true,
			errContains: "logger is not a valid ERC20 Transfer event",
		},
		{
			name: "Error - Too Many Topics",
			logger: types.Log{
				Topics: []common.Hash{transferEventID, fromTopic, toTopic, toTopic}, // Extra topic
				Data:   oneTokenData,
			},
			expectErr:   true,
			errContains: "logger is not a valid ERC20 Transfer event",
		},
		{
			name: "Error - Malformed Data Field",
			logger: types.Log{
				Topics: []common.Hash{transferEventID, fromTopic, toTopic},
				Data:   malformedData, // Data is too short
			},
			expectErr:   true,
			errContains: "failed to unpack logger data",
		},
	}

	// --- Run Tests ---
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sender, recipient, value, err := ParseERC20TransferEvent(tc.logger)

			if tc.expectErr {
				require.Error(t, err, "Expected an error but got none")
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains, "Error message does not contain expected text")
				}
			} else {
				require.NoError(t, err, "Did not expect an error but got one: %v", err)
				assert.Equal(t, tc.expectedFrom, sender, "Parsed sender address is incorrect")
				assert.Equal(t, tc.expectedTo, recipient, "Parsed recipient address is incorrect")
				require.NotNil(t, value, "Value should not be nil on success")
				assert.Zero(t, tc.expectedValue.Cmp(value), "Parsed value is incorrect")
			}
		})
	}
}
