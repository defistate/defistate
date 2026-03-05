package poolregistry

type TokenPoolRegistryDiff struct {
	Data *TokenPoolsRegistryView `json:"data,omitempty"`
}

// IsEmpty returns true if the diff contains no data.
func (d TokenPoolRegistryDiff) IsEmpty() bool {
	return d.Data == nil
}

// Differ is a concrete implementation of the TokenPoolRegistryDiffer function type.
//
// For this highly complex, graph-based subsystem, creating a minimal diff is a
// significant challenge that requires a dedicated graph-diffing algorithm.
//
// As a pragmatic, temporary solution, this function does not compute a minimal diff.
// Instead, it returns the complete, new view of the TokenPoolSystem. This ensures the
// end-to-end system can be built and tested, while clearly marking this component
// for future optimization.
//
// @todo Implement a true, minimal graph-based diffing algorithm for this subsystem
// to maximize bandwidth efficiency. This will likely involve returning a set of
// specific graph mutations (e.g., edges added/removed, nodes updated).
func TokenPoolsRegistryDiffer(old, new *TokenPoolsRegistryView) TokenPoolRegistryDiff {
	// For now, we simply return the new, full data structure.
	// The client-side logic will need to handle this by replacing its
	// existing TokenPoolSystem view wholesale when it receives this diff.
	return TokenPoolRegistryDiff{
		Data: new,
	}
}
