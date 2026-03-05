package poolregistry

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

var empty12Bytes = make([]byte, 12)

// --- PoolKey Implementation ---

// PoolKey is a fixed-size 32-byte container designed to hold any protocol's pool identifier.
//
// Motivation:
// While most DeFi protocol pools are identified by a 20-byte Ethereum address,
// other protocols (e.g., Balancer, Uniswap v4) use 32-byte identifiers (bytes32 hashes).
// PoolKey normalizes these diverse identifiers into a single, comparable, hashable type.
//
// Encoding rules:
//   - Address-based identifiers are stored in Ethereum ABI form:
//     [0..11] = zero padding, [12..31] = address (right-aligned)
//   - bytes32 identifiers are stored verbatim in [0..31]
//
// PoolKey MUST NOT be treated as a generic ABI word; conversions must be explicit.
type PoolKey [32]byte

// Bytes returns the raw underlying byte slice.
// Output: A 32-byte slice.
func (p PoolKey) Bytes() []byte {
	return p[:]
}

// String returns the hex string representation of the key.
// Output: A standard hex string starting with "0x".
func (p PoolKey) String() string {
	return "0x" + hex.EncodeToString(p[:])
}

// MarshalJSON serializes the key as a hex string.
func (p PoolKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// UnmarshalJSON parses a hex string into the key.
//
// Input:
//   - Hex string of even length
//   - Optional "0x" prefix
//
// Semantics:
//   - Decoded bytes are copied verbatim into the first N bytes of the PoolKey
//   - Remaining bytes are zero-padded
//
// Notes:
//   - This is appropriate for bytes32 identifiers that may be serialized
//     without leading zeros.
//   - This function does NOT perform ABI-aware decoding for addresses.
func (p *PoolKey) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s = strings.TrimPrefix(s, "0x")

	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) > 32 {
		return errors.New("pool key too long")
	}

	// Wipe existing data to prevent dirty reads if reusing the struct
	*p = PoolKey{}
	copy(p[:], b)

	return nil
}

// AddressToPoolKey converts an Ethereum address into a PoolKey.
//
// Encoding:
//   - Ethereum ABI–aligned
//   - Address is right-aligned in the 32-byte word
//
// Layout:
//
//	[0..11]  = 0x00 padding
//	[12..31] = address (20 bytes)
func AddressToPoolKey(addr common.Address) PoolKey {
	var key PoolKey
	copy(key[12:], addr[:])
	return key
}

// ToAddress attempts to interpret the PoolKey as an Ethereum address.
//
// This is a best-effort, ABI-shape-based conversion.
//
// Validation rule:
//   - The PoolKey is treated as an address only if the first 12 bytes are zero,
//     matching the Ethereum ABI encoding of an address in a 32-byte word
//     (left-padded with 12 zero bytes, right-aligned 20-byte address).
//
// Limitations:
//   - This check cannot prove that the PoolKey was originally derived from
//     a 20-byte address.
//   - A 32-byte identifier (e.g., a hash) with 12 leading zero bytes would
//     be misclassified as an address, though this is statistically negligible
//     for cryptographic hashes.
//
// Returns an error if the PoolKey does not conform to the ABI address shape.
func (p PoolKey) ToAddress() (common.Address, error) {
	// confirm that first 12 bytes are zero
	for _, b := range p[:12] {
		if b != 0 {
			return common.Address{}, errors.New("pool key is not an ABI-encoded Ethereum address")
		}
	}
	return common.Address(p[12:32]), nil
}

func Bytes32ToPoolKey(b [32]byte) PoolKey {
	return PoolKey(b)
}
