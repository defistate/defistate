package bitset

import "fmt"

func NewBitSet(len uint64) BitSet {
	words := (len + 63) / 64
	bits := make([]uint64, words)
	return bits
}

type BitSet []uint64

func (b BitSet) IsSet(index uint64) bool {
	wordPosition := index / 64
	bitPosition := index % 64
	mask := uint64(1) << bitPosition

	return (b[wordPosition] & mask) != 0
}

func (b BitSet) Set(index uint64) {
	wordPosition := index / 64
	bitPosition := index % 64
	mask := uint64(1) << bitPosition

	b[wordPosition] |= mask
}

func (b BitSet) Unset(index uint64) {
	wordPosition := index / 64
	bitPosition := index % 64
	mask := uint64(1) << bitPosition

	b[wordPosition] = b[wordPosition] &^ mask

}

func (b BitSet) Clear() {
	for i := range b {
		b[i] = 0
	}
}

func (b BitSet) SetFrom(o BitSet) {
	if len(b) != len(o) {
		panic(fmt.Sprintf("bitsets must be same size: got %d vs %d", len(b), len(o)))
	}
	copy(b, o)
}
