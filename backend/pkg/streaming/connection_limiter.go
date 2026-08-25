package streaming

import (
	"fmt"
	"sync"
)

// Source identifies one playable upstream and the playlist/provider budget it
// consumes. A zero PoolID is reserved for callers that cannot associate an
// upstream with a configured playlist and is therefore not globally limited.
type Source struct {
	URL             string
	PoolID          int64
	PoolName        string
	ConnectionLimit int
}

type ConnectionUsage struct {
	PoolID int64  `json:"source_id"`
	Name   string `json:"source_name"`
	Active int    `json:"active"`
	Limit  int    `json:"limit"`
}

type connectionLease struct {
	coordinator *connectionCoordinator
	poolID      int64
	once        sync.Once
}

func (lease *connectionLease) Release() {
	if lease == nil || lease.coordinator == nil || lease.poolID == 0 {
		return
	}
	lease.once.Do(func() { lease.coordinator.release(lease.poolID) })
}

type sourcePool struct {
	name   string
	active int
	limit  int
}

// connectionCoordinator is deliberately non-blocking. The stream supervisor
// decides whether to try another provider, stop a prewarm, or wait before the
// next round instead of letting goroutines pile up behind a saturated source.
type connectionCoordinator struct {
	mu    sync.Mutex
	pools map[int64]*sourcePool
}

func newConnectionCoordinator() *connectionCoordinator {
	return &connectionCoordinator{pools: make(map[int64]*sourcePool)}
}

func (c *connectionCoordinator) acquire(source Source) (*connectionLease, error) {
	if source.PoolID == 0 {
		return &connectionLease{}, nil
	}
	limit := source.ConnectionLimit
	if limit < 1 {
		limit = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pool := c.pools[source.PoolID]
	if pool == nil {
		pool = &sourcePool{}
		c.pools[source.PoolID] = pool
	}
	pool.name, pool.limit = source.PoolName, limit
	if pool.active >= limit {
		return nil, fmt.Errorf("source %q connection limit reached (%d/%d)", source.PoolName, pool.active, limit)
	}
	pool.active++
	return &connectionLease{coordinator: c, poolID: source.PoolID}, nil
}

func (c *connectionCoordinator) release(poolID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pool := c.pools[poolID]; pool != nil && pool.active > 0 {
		pool.active--
		if pool.active == 0 {
			delete(c.pools, poolID)
		}
	}
}

func (c *connectionCoordinator) usage() []ConnectionUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := make([]ConnectionUsage, 0, len(c.pools))
	for id, pool := range c.pools {
		items = append(items, ConnectionUsage{PoolID: id, Name: pool.name, Active: pool.active, Limit: pool.limit})
	}
	return items
}
