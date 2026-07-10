package bot

import (
	"container/list"
	"sync"

	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

// predCacheSize is the number of recent predictions kept in memory. The service
// always returns every label, so on an "are you sure" retry we can re-render
// from this cache (relaxing the confidence gate) instead of calling inference
// again. Older entries are evicted LRU-style once the buffer is full.
const predCacheSize = 32

// cachedPrediction is a full inference result plus the annotated image path we
// already wrote for it, keyed by the original message id.
type cachedPrediction struct {
	pred    *inference.PredictResponse
	outFile string // annotated image (or the input file) already sent
	inFile  string // the downloaded source image, for a fresh re-run if needed
}

// predCache is a small thread-safe LRU of recent predictions keyed by original
// message id. It is intentionally tiny and in-memory: a miss simply falls back
// to a fresh inference call, so losing entries on restart or eviction is fine.
type predCache struct {
	mu    sync.Mutex
	max   int
	ll    *list.List              // front = most recently used
	items map[int64]*list.Element // ogMessageID -> element holding *predEntry
}

type predEntry struct {
	key int64
	val cachedPrediction
}

func newPredCache(max int) *predCache {
	return &predCache{
		max:   max,
		ll:    list.New(),
		items: make(map[int64]*list.Element, max),
	}
}

// put inserts or refreshes the entry for ogMessageID, evicting the least
// recently used entry when over capacity.
func (c *predCache) put(ogMessageID int64, v cachedPrediction) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[ogMessageID]; ok {
		el.Value.(*predEntry).val = v
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&predEntry{key: ogMessageID, val: v})
	c.items[ogMessageID] = el

	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*predEntry).key)
	}
}

// get returns the cached prediction for ogMessageID and marks it most recently
// used. ok is false on a miss.
func (c *predCache) get(ogMessageID int64) (cachedPrediction, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[ogMessageID]
	if !ok {
		return cachedPrediction{}, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*predEntry).val, true
}
