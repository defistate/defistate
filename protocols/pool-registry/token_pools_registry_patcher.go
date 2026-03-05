package poolregistry

// deepCopyView creates a new TokenPoolsRegistryView with its own memory for all its slices.
// This is essential to prevent the new state from sharing memory with the diff object.
func deepCopyView(v *TokenPoolsRegistryView) *TokenPoolsRegistryView {
	if v == nil {
		return nil
	}
	newV := &TokenPoolsRegistryView{}
	// Use the full slice expression `[:]` to create a copy of the slice's backing array.
	newV.Tokens = append(make([]uint64, 0, len(v.Tokens)), v.Tokens...)
	newV.Pools = append(make([]uint64, 0, len(v.Pools)), v.Pools...)
	newV.EdgeTargets = append(make([]int, 0, len(v.EdgeTargets)), v.EdgeTargets...)

	if v.Adjacency != nil {
		newV.Adjacency = make([][]int, len(v.Adjacency))
		for i, inner := range v.Adjacency {
			newV.Adjacency[i] = append(make([]int, 0, len(inner)), inner...)
		}
	}
	if v.EdgePools != nil {
		newV.EdgePools = make([][]int, len(v.EdgePools))
		for i, inner := range v.EdgePools {
			newV.EdgePools[i] = append(make([]int, 0, len(inner)), inner...)
		}
	}
	return newV
}

// Patcher is a concrete implementation of the TokenPoolRegistryPatcher function type.
// As a pragmatic temporary solution, it ignores the prevState and simply returns a
// deep copy of the new, full state provided in the diff's Data field.
// @todo Replace this with a true graph-based patching algorithm once the differ is updated.
func TokenPoolRegistryPatcher(prevState *TokenPoolsRegistryView, diff TokenPoolRegistryDiff) (*TokenPoolsRegistryView, error) {
	// The core of the temporary strategy: return a deep copy of the new data.
	return deepCopyView(diff.Data), nil
}
