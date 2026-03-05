package poolregistry

// TokenPoolsRegistryView provides a complete, thread-safe snapshot of the graph's
// core data structures. This is optimized for consumers who need to perform
// their own graph traversal or analysis algorithms.
type TokenPoolsRegistryView struct {
	Tokens      []uint64 `json:"tokens"`
	Pools       []uint64 `json:"pools"`
	Adjacency   [][]int  `json:"adjacency"`
	EdgeTargets []int    `json:"edgeTargets"`
	EdgePools   [][]int  `json:"edgePools"`
}

// TokenPoolsRegistry is a simple, non-thread-safe data structure that manages
// the relationship between tokens and pools using a graph representation for high performance.
type TokenPoolsRegistry struct {
	// Lookups for fast index retrieval
	tokenToIndex map[uint64]int
	poolToIndex  map[uint64]int

	// Core data stored in slices for cache-friendly access (DOD)
	tokens              []uint64
	pools               []uint64
	adjacency           [][]int
	edgeTargets         []int
	edgePools           [][]int
	danglingEdgeCount   int
	compactionThreshold int
}

// NewTokenPoolsRegistry creates a new, properly initialized graph-based registry.
func NewTokenPoolsRegistry(compactionThreshold int) *TokenPoolsRegistry {
	if compactionThreshold <= 0 {
		compactionThreshold = 1000
	}
	return &TokenPoolsRegistry{
		tokenToIndex:        make(map[uint64]int),
		poolToIndex:         make(map[uint64]int),
		tokens:              make([]uint64, 0),
		pools:               make([]uint64, 0),
		adjacency:           make([][]int, 0),
		edgeTargets:         make([]int, 0),
		edgePools:           make([][]int, 0),
		compactionThreshold: compactionThreshold,
	}
}

// NewTokenPoolsRegistryFromView reconstructs a registry from a view snapshot.
// This is the primary mechanism for restoring the graph's state. It creates a
// deep copy of the view data to ensure the new registry has full ownership of its memory.
func NewTokenPoolsRegistryFromView(view *TokenPoolsRegistryView, compactionThreshold int) *TokenPoolsRegistry {
	if compactionThreshold <= 0 {
		compactionThreshold = 1000 // Default value
	}

	// Pre-allocate maps with the correct size.
	tokenToIndex := make(map[uint64]int, len(view.Tokens))
	poolToIndex := make(map[uint64]int, len(view.Pools))

	// Rebuild the lookup maps.
	for i, tokenID := range view.Tokens {
		tokenToIndex[tokenID] = i
	}
	for i, poolID := range view.Pools {
		poolToIndex[poolID] = i
	}

	// Perform a deep copy of all slice data.
	tokensCopy := make([]uint64, len(view.Tokens))
	copy(tokensCopy, view.Tokens)

	poolsCopy := make([]uint64, len(view.Pools))
	copy(poolsCopy, view.Pools)

	adjacencyCopy := make([][]int, len(view.Adjacency))
	for i, adj := range view.Adjacency {
		// CORRECTED LOGIC: Preserve nil slices correctly.
		if adj == nil {
			adjacencyCopy[i] = nil
		} else {
			adjCopy := make([]int, len(adj))
			copy(adjCopy, adj)
			adjacencyCopy[i] = adjCopy
		}
	}

	edgeTargetsCopy := make([]int, len(view.EdgeTargets))
	copy(edgeTargetsCopy, view.EdgeTargets)

	edgePoolsCopy := make([][]int, len(view.EdgePools))
	for i, poolList := range view.EdgePools {
		// CORRECTED LOGIC: Preserve nil slices correctly.
		if poolList == nil {
			edgePoolsCopy[i] = nil
		} else {
			listCopy := make([]int, len(poolList))
			copy(listCopy, poolList)
			edgePoolsCopy[i] = listCopy
		}
	}

	return &TokenPoolsRegistry{
		tokenToIndex:        tokenToIndex,
		poolToIndex:         poolToIndex,
		tokens:              tokensCopy,
		pools:               poolsCopy,
		adjacency:           adjacencyCopy,
		edgeTargets:         edgeTargetsCopy,
		edgePools:           edgePoolsCopy,
		danglingEdgeCount:   0, // A restored view should be clean with no dangling edges.
		compactionThreshold: compactionThreshold,
	}
}

// addEdge creates or updates a directed edge from a source token to a target token,
// associating it with the given pool.
func (r *TokenPoolsRegistry) addEdge(fromTokenID, toTokenID, poolID uint64) {
	fromIndex, exists := r.tokenToIndex[fromTokenID]
	if !exists {
		fromIndex = len(r.tokens)
		r.tokens = append(r.tokens, fromTokenID)
		r.tokenToIndex[fromTokenID] = fromIndex
		r.adjacency = append(r.adjacency, nil)
	}
	toTokenIndex, exists := r.tokenToIndex[toTokenID]
	if !exists {
		toTokenIndex = len(r.tokens)
		r.tokens = append(r.tokens, toTokenID)
		r.tokenToIndex[toTokenID] = toTokenIndex
		r.adjacency = append(r.adjacency, nil)
	}
	poolIndex, exists := r.poolToIndex[poolID]
	if !exists {
		poolIndex = len(r.pools)
		r.pools = append(r.pools, poolID)
		r.poolToIndex[poolID] = poolIndex
	}

	// Search for an existing edge from the source to the target token.
	for _, edgeIndex := range r.adjacency[fromIndex] {
		if r.edgeTargets[edgeIndex] == toTokenIndex {
			// Edge already exists. Check if the pool is already associated with it.
			for _, existingPoolIndex := range r.edgePools[edgeIndex] {
				if existingPoolIndex == poolIndex {
					return // Pool is already associated with this edge.
				}
			}
			// Add the new pool to the existing edge's pool list.
			r.edgePools[edgeIndex] = append(r.edgePools[edgeIndex], poolIndex)
			return
		}
	}

	// If no edge exists, create a new one.
	newEdgeIndex := len(r.edgeTargets)
	r.edgeTargets = append(r.edgeTargets, toTokenIndex)
	// Initialize the pool list for the new edge.
	r.edgePools = append(r.edgePools, []int{poolIndex})
	r.adjacency[fromIndex] = append(r.adjacency[fromIndex], newEdgeIndex)
}

// add creates a fully connected graph (a clique) between all tokens in the pool.
func (r *TokenPoolsRegistry) add(tokenIDs []uint64, poolID uint64) {
	for i := 0; i < len(tokenIDs); i++ {
		for j := i + 1; j < len(tokenIDs); j++ {
			tokenA := tokenIDs[i]
			tokenB := tokenIDs[j]
			r.addEdge(tokenA, tokenB, poolID)
			r.addEdge(tokenB, tokenA, poolID)
		}
	}
}

// removePool performs a logical deletion of a pool by removing it from all edges.
// If an edge's pool list becomes empty, that edge is marked as dangling.
func (r *TokenPoolsRegistry) removePool(poolID uint64) {
	poolIndexToRemove, exists := r.poolToIndex[poolID]
	if !exists {
		return
	}

	for edgeIndex, poolList := range r.edgePools {
		if len(poolList) == 0 {
			continue
		}

		// Build a new list containing all pools except the one to be removed.
		newPoolList := poolList[:0] // In-place slice reuse
		wasRemoved := false
		for _, pIndex := range poolList {
			if pIndex != poolIndexToRemove {
				newPoolList = append(newPoolList, pIndex)
			} else {
				wasRemoved = true
			}
		}

		// If the pool was found and removed, update the list.
		if wasRemoved {
			r.edgePools[edgeIndex] = newPoolList
			// If the list is now empty, the edge is dangling.
			if len(newPoolList) == 0 {
				r.danglingEdgeCount++
			}
		}
	}

	if r.danglingEdgeCount > r.compactionThreshold {
		r.compact()
	}
}

// removeToken performs a logical deletion of a token by marking all its associated edges as dangling.
func (r *TokenPoolsRegistry) removeToken(tokenID uint64) {
	tokenIndexToRemove, exists := r.tokenToIndex[tokenID]
	if !exists {
		return
	}

	// Mark all outgoing edges as dangling by clearing their pool lists.
	for _, edgeIndex := range r.adjacency[tokenIndexToRemove] {
		if len(r.edgePools[edgeIndex]) > 0 {
			r.edgePools[edgeIndex] = nil
			r.danglingEdgeCount++
		}
	}
	r.adjacency[tokenIndexToRemove] = nil

	// Mark all incoming edges as dangling.
	for edgeIndex, targetIndex := range r.edgeTargets {
		if targetIndex == tokenIndexToRemove {
			if len(r.edgePools[edgeIndex]) > 0 {
				r.edgePools[edgeIndex] = nil
				r.danglingEdgeCount++
			}
		}
	}

	if r.danglingEdgeCount > r.compactionThreshold {
		r.compact()
	}
}

// compact rebuilds all internal data structures to physically remove dangling entries.
func (r *TokenPoolsRegistry) compact() {
	if r.danglingEdgeCount == 0 {
		return
	}

	// Step 1: Compact edges by removing those with no associated pools.
	oldToNewEdgeIndex := make(map[int]int, len(r.edgeTargets)-r.danglingEdgeCount)
	newEdgeTargets := make([]int, 0, len(r.edgeTargets))
	newEdgePools := make([][]int, 0, len(r.edgePools))

	for readIdx, poolList := range r.edgePools {
		if len(poolList) > 0 { // Is this a live edge?
			newIdx := len(newEdgeTargets)
			oldToNewEdgeIndex[readIdx] = newIdx
			newEdgeTargets = append(newEdgeTargets, r.edgeTargets[readIdx])
			newEdgePools = append(newEdgePools, poolList)
		}
	}

	// Step 2: Identify all tokens and pools still referenced by live edges.
	usedTokens := make(map[int]struct{})
	usedPools := make(map[int]struct{})

	for _, tokenIndex := range newEdgeTargets {
		usedTokens[tokenIndex] = struct{}{}
	}
	for i, adj := range r.adjacency {
		for _, oldEdgeIdx := range adj {
			if _, ok := oldToNewEdgeIndex[oldEdgeIdx]; ok {
				usedTokens[i] = struct{}{}
				break
			}
		}
	}
	for _, poolList := range newEdgePools {
		for _, poolIndex := range poolList {
			usedPools[poolIndex] = struct{}{}
		}
	}

	// Step 3: Compact tokens and create a remapping table.
	oldToNewTokenIndex := make(map[int]int, len(usedTokens))
	finalTokens := make([]uint64, 0, len(usedTokens))
	finalTokenToIndex := make(map[uint64]int, len(usedTokens))
	for oldIdx, tokenID := range r.tokens {
		if _, ok := usedTokens[oldIdx]; ok {
			newIdx := len(finalTokens)
			oldToNewTokenIndex[oldIdx] = newIdx
			finalTokens = append(finalTokens, tokenID)
			finalTokenToIndex[tokenID] = newIdx
		}
	}

	// Step 4: Compact pools and create a remapping table.
	oldToNewPoolIndex := make(map[int]int, len(usedPools))
	finalPools := make([]uint64, 0, len(usedPools))
	finalPoolToIndex := make(map[uint64]int, len(usedPools))
	for oldIdx, poolID := range r.pools {
		if _, ok := usedPools[oldIdx]; ok {
			newIdx := len(finalPools)
			oldToNewPoolIndex[oldIdx] = newIdx
			finalPools = append(finalPools, poolID)
			finalPoolToIndex[poolID] = newIdx
		}
	}

	// Step 5: Remap indices within the already-compacted edge and pool slices.
	for i := range newEdgeTargets {
		newEdgeTargets[i] = oldToNewTokenIndex[newEdgeTargets[i]]
	}
	for i, poolList := range newEdgePools {
		for j, oldPoolIdx := range poolList {
			newEdgePools[i][j] = oldToNewPoolIndex[oldPoolIdx]
		}
	}

	// Step 6: Rebuild the adjacency list from scratch using the new indices.
	finalAdjacency := make([][]int, len(finalTokens))
	for oldTokenIdx, oldAdj := range r.adjacency {
		if newTokenIdx, ok := oldToNewTokenIndex[oldTokenIdx]; ok {
			newAdj := make([]int, 0, len(oldAdj))
			for _, oldEdgeIdx := range oldAdj {
				if newEdgeIdx, ok := oldToNewEdgeIndex[oldEdgeIdx]; ok {
					newAdj = append(newAdj, newEdgeIdx)
				}
			}
			finalAdjacency[newTokenIdx] = newAdj
		}
	}

	// Step 7: Atomically replace all old data structures with the new ones.
	r.tokens = finalTokens
	r.tokenToIndex = finalTokenToIndex
	r.pools = finalPools
	r.poolToIndex = finalPoolToIndex
	r.edgeTargets = newEdgeTargets
	r.edgePools = newEdgePools
	r.adjacency = finalAdjacency
	r.danglingEdgeCount = 0
}

func (r *TokenPoolsRegistry) poolsForToken(tokenID uint64) []uint64 {
	tokenIndex, exists := r.tokenToIndex[tokenID]
	if !exists {
		return nil
	}

	// Use a map to collect unique pool IDs to avoid duplicates.
	// E.g., if token A is in a pool with B and also a pool with C,
	// we don't want to list the same pool twice.
	uniquePools := make(map[uint64]struct{})

	// 1. Get all edge indices for the given token.
	edgeIndices := r.adjacency[tokenIndex]
	for _, edgeIndex := range edgeIndices {
		// 2. For each edge, get the list of associated pool indices.
		poolIndices := r.edgePools[edgeIndex]
		for _, poolIndex := range poolIndices {
			// 3. Use the pool index to look up the actual pool ID.
			if poolIndex < len(r.pools) { // Safety check
				poolID := r.pools[poolIndex]
				uniquePools[poolID] = struct{}{}
			}
		}
	}

	// Convert the map keys to a slice for the return value.
	if len(uniquePools) == 0 {
		return nil
	}
	poolIDs := make([]uint64, 0, len(uniquePools))
	for poolID := range uniquePools {
		poolIDs = append(poolIDs, poolID)
	}

	return poolIDs
}

// view returns a deep copy of the graph's core data structures.
func (r *TokenPoolsRegistry) view() *TokenPoolsRegistryView {
	tokensCopy := make([]uint64, len(r.tokens))
	copy(tokensCopy, r.tokens)

	poolsCopy := make([]uint64, len(r.pools))
	copy(poolsCopy, r.pools)

	adjacencyCopy := make([][]int, len(r.adjacency))
	for i, adj := range r.adjacency {
		adjCopy := make([]int, len(adj))
		copy(adjCopy, adj)
		adjacencyCopy[i] = adjCopy
	}

	edgeTargetsCopy := make([]int, len(r.edgeTargets))
	copy(edgeTargetsCopy, r.edgeTargets)

	// Updated deep copy logic for the new structure.
	edgePoolsCopy := make([][]int, len(r.edgePools))
	for i, poolList := range r.edgePools {
		listCopy := make([]int, len(poolList))
		copy(listCopy, poolList)
		edgePoolsCopy[i] = listCopy
	}

	return &TokenPoolsRegistryView{
		Tokens:      tokensCopy,
		Pools:       poolsCopy,
		Adjacency:   adjacencyCopy,
		EdgeTargets: edgeTargetsCopy,
		EdgePools:   edgePoolsCopy,
	}
}
