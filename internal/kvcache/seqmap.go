package kvcache

import "sync"

type SeqMap struct {
	mu sync.Mutex
	m  map[SeqID][]BlockID
}

func NewSeqMap() *SeqMap {
	return &SeqMap{m: make(map[SeqID][]BlockID)}
}

func (s *SeqMap) Get(id SeqID) []BlockID {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.m[id]
	if !ok {
		return nil
	}
	out := make([]BlockID, len(src))
	copy(out, src)
	return out
}

func (s *SeqMap) Set(id SeqID, blocks []BlockID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]BlockID, len(blocks))
	copy(cp, blocks)
	s.m[id] = cp
}

func (s *SeqMap) Append(id SeqID, blocks []BlockID) {
	if len(blocks) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = append(s.m[id], blocks...)
}

func (s *SeqMap) Remove(id SeqID) []BlockID {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocks := s.m[id]
	delete(s.m, id)
	out := make([]BlockID, len(blocks))
	copy(out, blocks)
	return out
}

func (s *SeqMap) Has(id SeqID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[id]
	return ok
}

func (s *SeqMap) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}
