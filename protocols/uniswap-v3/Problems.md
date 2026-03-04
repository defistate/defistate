**Missed Logs**
- Missing any logs for a block sends the system and ticks indexer into an inconsistent state 
- This happens if:
    1. We call getLogsWithRetry and we get empty for a block with actual logs (node is still indexing logs for block)
- Fixes:
    1. Our Resync Handler helps mitigate these problems, but we will still be inconsistent until the resync mechanism runs  
    2. Guarantee that we will fetch logs only when they have been indexed
    3. Fetch full state on every block (expensive but solves the problem completely if full state can be fetched before next block)



**Reorgs**
- A reorg will leave our systems in an inconsistent state because we made updates for state is no longer true
- Fixes: 
    1. Our Resync Handler helps mitigate these problems, but we will still be inconsistent until the resync mechanism runs
    2. Fetch full state on every block (expensive but solves the problem completely if full state can be fetched before next block)

