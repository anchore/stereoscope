package tree

import (
	"sync"

	"github.com/anchore/stereoscope/pkg/file"
)

// ReferencePool is a pool for deduplicating file references
type ReferencePool struct {
	refs []*file.Reference
	mu   sync.RWMutex
}

func NewReferencePool() *ReferencePool {
	return &ReferencePool{
		refs: make([]*file.Reference, 0),
	}
}

// Add returns the index for the given reference.
// If an equivalent reference already exists in the pool, the existing index is returned.
// If the reference is nil, returns 0.
func (p *ReferencePool) Add(ref *file.Reference) uint32 {
	if ref == nil {
		return 0
	}

	p.mu.RLock()
	// Try to find existing reference by ID
	for i, r := range p.refs {
		if r != nil && r.ID() == ref.ID() {
			p.mu.RUnlock()
			return uint32(i + 1)
		}
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check again since another goroutine might have added it
	for i, r := range p.refs {
		if r != nil && r.ID() == ref.ID() {
			return uint32(i + 1)
		}
	}

	idx := len(p.refs)
	p.refs = append(p.refs, ref)
	return uint32(idx + 1)
}

// Get returns the reference at the given index.
// If idx is 0, returns nil.
func (p *ReferencePool) Get(idx uint32) *file.Reference {
	if idx == 0 {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if int(idx-1) < len(p.refs) {
		return p.refs[idx-1]
	}
	return nil
}

// Len returns the number of references in the pool
func (p *ReferencePool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.refs)
}

// Copy returns a copy of the reference pool
func (p *ReferencePool) Copy() *ReferencePool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	newPool := NewReferencePool()
	newPool.refs = make([]*file.Reference, len(p.refs))
	copy(newPool.refs, p.refs)
	return newPool
}
