package tree

import (
	"sync"
)

// StringPool is a simple interner for strings to avoid duplications
type StringPool struct {
	strings []string
	index   map[string]uint32
	mu      sync.RWMutex
}

func NewStringPool() *StringPool {
	return &StringPool{
		strings: make([]string, 0),
		index:   make(map[string]uint32),
	}
}

// Intern returns a canonical index for the given input string.
// If the string already exists in the pool, the existing index is returned.
// Otherwise, the string is added to the pool and its new index is returned.
func (p *StringPool) Intern(s string) uint32 {
	p.mu.RLock()
	if idx, ok := p.index[s]; ok {
		p.mu.RUnlock()
		return idx
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check again since another goroutine might have added it while we released the read lock
	if idx, ok := p.index[s]; ok {
		return idx
	}

	idx := uint32(len(p.strings))
	p.strings = append(p.strings, s)
	p.index[s] = idx
	return idx
}

// Get returns the string at the given index.
func (p *StringPool) Get(idx uint32) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if int(idx) < len(p.strings) {
		return p.strings[idx]
	}
	return ""
}

// Len returns the number of unique strings in the pool
func (p *StringPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.strings)
}

// Copy returns a copy of the string pool
func (p *StringPool) Copy() *StringPool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	newPool := NewStringPool()
	newPool.strings = make([]string, len(p.strings))
	copy(newPool.strings, p.strings)
	for k, v := range p.index {
		newPool.index[k] = v
	}
	return newPool
}
