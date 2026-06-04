package kvcache

import (
	"fmt"
	"sync"
)

type Manager struct {
	mu       sync.Mutex
	opts     ManagerOptions
	refcount []int32
	inFree   []bool
	free     []BlockID
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if opts.NumBlocks <= 0 {
		return nil, fmt.Errorf("%w: NumBlocks must be > 0", ErrInvalidCount)
	}
	if opts.BlockSize <= 0 {
		return nil, fmt.Errorf("%w: BlockSize must be > 0", ErrInvalidCount)
	}
	m := &Manager{
		opts:     opts,
		refcount: make([]int32, opts.NumBlocks),
		inFree:   make([]bool, opts.NumBlocks),
		free:     make([]BlockID, opts.NumBlocks),
	}
	for i := 0; i < opts.NumBlocks; i++ {
		m.free[i] = BlockID(opts.NumBlocks - 1 - i)
		m.inFree[m.free[i]] = true
	}
	return m, nil
}

func (m *Manager) BlockSize() int { return m.opts.BlockSize }

func (m *Manager) Capacity() int { return m.opts.NumBlocks }

func (m *Manager) Allocate(n int) ([]BlockID, error) {
	if n < 0 {
		return nil, ErrInvalidCount
	}
	if n == 0 {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > len(m.free) {
		return nil, ErrCacheFull
	}
	out := make([]BlockID, n)
	for i := 0; i < n; i++ {
		id := m.free[len(m.free)-1]
		m.free = m.free[:len(m.free)-1]
		m.refcount[id] = 1
		m.inFree[id] = false
		out[i] = id
	}
	return out, nil
}

func (m *Manager) Free(ids []BlockID) error {
	if len(ids) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if err := m.unrefLocked(id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Ref(id BlockID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if int(id) >= len(m.refcount) {
		return ErrInvalidBlockID
	}
	if m.refcount[id] <= 0 {
		return ErrRefcountUnderflow
	}
	m.refcount[id]++
	return nil
}

func (m *Manager) Unref(id BlockID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unrefLocked(id)
}

func (m *Manager) unrefLocked(id BlockID) error {
	if int(id) >= len(m.refcount) {
		return ErrInvalidBlockID
	}
	if m.refcount[id] <= 0 {
		return ErrRefcountUnderflow
	}
	m.refcount[id]--
	if m.refcount[id] == 0 {
		m.inFree[id] = true
		m.free = append(m.free, id)
	}
	return nil
}

func (m *Manager) Refcount(id BlockID) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if int(id) >= len(m.refcount) {
		return -1
	}
	return int(m.refcount[id])
}

func (m *Manager) FreeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.free)
}

func (m *Manager) UsedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.refcount) - len(m.free)
}
