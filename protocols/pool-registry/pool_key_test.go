package poolregistry

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolKey(t *testing.T) {
	// Standard Ethereum address (20 bytes)
	addrHex := "0x0000000000000000000000000000000000000001"
	addr := common.HexToAddress(addrHex)

	// 32-byte hash
	hashHex := "0x0000000000000000000000000000000000000000000000000000000000000002"
	hash := common.HexToHash(hashHex)
	hashBytes := hash.Bytes()
	require.Len(t, hashBytes, 32)

	// Convert to [32]byte for Bytes32ToPoolKey
	var hashArr [32]byte
	copy(hashArr[:], hashBytes)

	t.Run("AddressToPoolKey_ABIAligned", func(t *testing.T) {
		key := AddressToPoolKey(addr)

		// ABI layout for address in a 32-byte word:
		// [0..11]  = 0x00 padding
		// [12..31] = address (20 bytes)
		assert.Equal(t, make([]byte, 12), key[:12], "first 12 bytes should be zero padding")
		assert.Equal(t, addr.Bytes(), key[12:32], "last 20 bytes should match address")

		// Round-trip extraction
		gotAddr, err := key.ToAddress()
		require.NoError(t, err)
		assert.Equal(t, addr, gotAddr, "ToAddress should round-trip the original address")

		// Verify string representation
		str := key.String()
		assert.Len(t, str, 66, "string representation should be 66 chars (0x + 64 hex)")
		assert.Equal(t, "0x"+common.Bytes2Hex(key[:]), str)
	})

	t.Run("Bytes32ToPoolKey_FromHash", func(t *testing.T) {
		key := Bytes32ToPoolKey(hashArr)

		assert.Equal(t, hashBytes, key[:], "key should exactly match the 32-byte hash")
		assert.Equal(t, hashHex, key.String(), "string representation should match original hex")
	})

	t.Run("ToAddress_RejectsNonABIShape", func(t *testing.T) {
		// Use a bytes32 value that does NOT have 12 leading zeros.
		var b [32]byte
		b[0] = 0xFF

		key := Bytes32ToPoolKey(b)

		_, err := key.ToAddress()
		assert.Error(t, err, "should fail if PoolKey does not match ABI-encoded address shape")
	})

	t.Run("JSON_Marshaling_RoundTrip", func(t *testing.T) {
		key := AddressToPoolKey(addr)

		// Marshal to JSON
		jsonBytes, err := key.MarshalJSON()
		require.NoError(t, err)

		// Expecting a JSON string: "0x<64-hex-chars>"
		expectedJSON := `"` + key.String() + `"`
		assert.Equal(t, expectedJSON, string(jsonBytes), "JSON output should be a hex string")

		// Unmarshal back
		var decodedKey PoolKey
		err = decodedKey.UnmarshalJSON(jsonBytes)
		require.NoError(t, err)
		assert.Equal(t, key, decodedKey, "decoded key should match original")
	})

	t.Run("JSON_Unmarshal_Validation", func(t *testing.T) {
		var k PoolKey

		// Invalid hex
		err := k.UnmarshalJSON([]byte(`"0xZZZ"`))
		assert.Error(t, err, "should fail on invalid hex")

		// Not a string
		err = k.UnmarshalJSON([]byte(`123`))
		assert.Error(t, err, "should fail on non-string JSON")

		// Too long (> 32 bytes): 33 bytes => 66 hex bytes after 0x prefix
		tooLong := `"0x` + strings.Repeat("00", 33) + `"`
		err = k.UnmarshalJSON([]byte(tooLong))
		assert.Error(t, err, "should fail if input is > 32 bytes")
	})

	t.Run("JSON_Unmarshal_ShorterInput_LeftCopies", func(t *testing.T) {
		// Captures current UnmarshalJSON semantics:
		// shorter inputs are copied into the front and right-padded with zeros.
		var k PoolKey

		err := k.UnmarshalJSON([]byte(`"0x0102"`)) // 2 bytes
		require.NoError(t, err)

		assert.Equal(t, byte(0x01), k[0])
		assert.Equal(t, byte(0x02), k[1])
		assert.Equal(t, make([]byte, 30), k[2:], "remaining bytes should be zero")
	})
}
