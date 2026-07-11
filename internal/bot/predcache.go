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
const predCacheSize = 8

// predKey identifies one cached image prediction: the original message id plus
// the specific attachment id. A message with several images has several
// distinct entries, so an "are you sure" retry re-renders every image rather
// than only the last one processed (the old message-id-only key collided).
type predKey struct {
	ogMessageID  int64
	attachmentID int64
}

// cachedPrediction is a full inference result plus the annotated image path we
// already wrote for it, keyed by (message id, attachment id).
type cachedPrediction struct {
	pred    *inference.PredictResponse
	outFile string // annotated image (or the input file) already sent
	inFile  string // the downloaded source image, for a fresh re-run if needed
}

// predCache is a small thread-safe LRU of recent predictions keyed by
// (message id, attachment id). It is intentionally tiny and in-memory: a miss
// simply falls back to a fresh inference call, so losing entries on restart or
// eviction is fine.
type predCache struct {
	mu    sync.Mutex
	max   int
	ll    *list.List                // front = most recently used
	items map[predKey]*list.Element // key -> element holding *predEntry
}

type predEntry struct {
	key predKey
	val cachedPrediction
}

func newPredCache(max int) *predCache {
	return &predCache{
		max:   max,
		ll:    list.New(),
		items: make(map[predKey]*list.Element, max),
	}
}

// put inserts or refreshes the entry for key, evicting the least recently used
// entry when over capacity.
func (c *predCache) put(key predKey, v cachedPrediction) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		el.Value.(*predEntry).val = v
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&predEntry{key: key, val: v})
	c.items[key] = el

	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*predEntry).key)
	}
}

// get returns the cached prediction for key and marks it most recently used.
// ok is false on a miss.
func (c *predCache) get(key predKey) (cachedPrediction, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return cachedPrediction{}, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*predEntry).val, true
}
