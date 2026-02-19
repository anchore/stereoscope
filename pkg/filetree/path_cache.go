package filetree

import (
	"sync"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

type lruEntry struct {
	nodeID node.ID
	path   string
	next   *lruEntry
	prev   *lruEntry
}

type LRUCache struct {
	maxSize int
	mu      sync.RWMutex
	head    *lruEntry
	tail    *lruEntry
	lookup  map[node.ID]*lruEntry
	size    int
}

func NewLRUCache(maxSize int) *LRUCache {
	if maxSize <= 0 {
		maxSize = 1000
	}

	cache := &LRUCache{
		maxSize: maxSize,
		lookup:  make(map[node.ID]*lruEntry),
	}

	cache.head = &lruEntry{}
	cache.tail = &lruEntry{}
	cache.head.next = cache.tail
	cache.tail.prev = cache.head

	return cache
}

func (c *LRUCache) Get(id node.ID) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.lookup[id]; ok {
		c.moveToFront(entry)
		return entry.path, true
	}

	return "", false
}

func (c *LRUCache) Put(id node.ID, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.lookup[id]; ok {
		entry.path = path
		c.moveToFront(entry)
		return
	}

	if c.size >= c.maxSize {
		c.removeOldest()
	}

	entry := &lruEntry{
		nodeID: id,
		path:   path,
	}

	c.addToFront(entry)
	c.lookup[id] = entry
	c.size++
}

func (c *LRUCache) moveToFront(entry *lruEntry) {
	entry.prev.next = entry.next
	entry.next.prev = entry.prev

	entry.prev = c.head
	entry.next = c.head.next
	c.head.next.prev = entry
	c.head.next = entry
}

func (c *LRUCache) addToFront(entry *lruEntry) {
	entry.prev = c.head
	entry.next = c.head.next
	c.head.next.prev = entry
	c.head.next = entry
}

func (c *LRUCache) removeOldest() {
	if c.size == 0 {
		return
	}

	oldest := c.tail.prev
	oldest.prev.next = oldest.next
	oldest.next.prev = oldest.prev

	delete(c.lookup, oldest.nodeID)
	c.size--
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lookup = make(map[node.ID]*lruEntry)
	c.head.next = c.tail
	c.tail.prev = c.head
	c.size = 0
}
