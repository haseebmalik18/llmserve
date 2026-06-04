package kvcache

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
)

func mustNewManager(t *testing.T, blockSize, numBlocks int) *Manager {
	t.Helper()
	m, err := NewManager(ManagerOptions{BlockSize: blockSize, NumBlocks: numBlocks})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestAllocateAndFree(t *testing.T) {
	m := mustNewManager(t, 16, 8)

	ids, err := m.Allocate(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("got %d ids, want 3", len(ids))
	}
	if m.UsedCount() != 3 {
		t.Fatalf("UsedCount: got %d, want 3", m.UsedCount())
	}
	if m.FreeCount() != 5 {
		t.Fatalf("FreeCount: got %d, want 5", m.FreeCount())
	}
	for _, id := range ids {
		if m.Refcount(id) != 1 {
			t.Fatalf("Refcount(%d): got %d, want 1", id, m.Refcount(id))
		}
	}

	if err := m.Free(ids); err != nil {
		t.Fatal(err)
	}
	if m.UsedCount() != 0 {
		t.Fatalf("UsedCount after Free: %d", m.UsedCount())
	}
	if m.FreeCount() != 8 {
		t.Fatalf("FreeCount after Free: %d", m.FreeCount())
	}
}

func TestAllocateZeroReturnsNil(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	ids, err := m.Allocate(0)
	if err != nil {
		t.Fatal(err)
	}
	if ids != nil {
		t.Fatalf("got %v, want nil", ids)
	}
}

func TestAllocateBeyondCapacity(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	if _, err := m.Allocate(5); !errors.Is(err, ErrCacheFull) {
		t.Fatalf("got %v, want ErrCacheFull", err)
	}
	ids, err := m.Allocate(4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Allocate(1); !errors.Is(err, ErrCacheFull) {
		t.Fatalf("got %v, want ErrCacheFull", err)
	}
	_ = ids
}

func TestRefSharedBlock(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	ids, _ := m.Allocate(1)
	id := ids[0]

	if err := m.Ref(id); err != nil {
		t.Fatal(err)
	}
	if rc := m.Refcount(id); rc != 2 {
		t.Fatalf("Refcount after Ref: %d, want 2", rc)
	}

	if err := m.Unref(id); err != nil {
		t.Fatal(err)
	}
	if rc := m.Refcount(id); rc != 1 {
		t.Fatalf("Refcount after Unref: %d, want 1", rc)
	}
	if m.UsedCount() != 1 {
		t.Fatalf("block should still be in use; UsedCount=%d", m.UsedCount())
	}

	if err := m.Unref(id); err != nil {
		t.Fatal(err)
	}
	if m.UsedCount() != 0 {
		t.Fatalf("block should now be free; UsedCount=%d", m.UsedCount())
	}
}

func TestRefOnFreeBlockFails(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	if err := m.Ref(BlockID(0)); !errors.Is(err, ErrRefcountUnderflow) {
		t.Fatalf("got %v, want ErrRefcountUnderflow", err)
	}
}

func TestDoubleUnrefFails(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	ids, _ := m.Allocate(1)
	id := ids[0]
	if err := m.Unref(id); err != nil {
		t.Fatal(err)
	}
	if err := m.Unref(id); !errors.Is(err, ErrRefcountUnderflow) {
		t.Fatalf("got %v, want ErrRefcountUnderflow", err)
	}
}

func TestInvalidBlockID(t *testing.T) {
	m := mustNewManager(t, 16, 4)
	if err := m.Ref(BlockID(99)); !errors.Is(err, ErrInvalidBlockID) {
		t.Fatalf("got %v, want ErrInvalidBlockID", err)
	}
	if err := m.Unref(BlockID(99)); !errors.Is(err, ErrInvalidBlockID) {
		t.Fatalf("got %v, want ErrInvalidBlockID", err)
	}
}

func TestReuseAfterFree(t *testing.T) {
	m := mustNewManager(t, 16, 2)
	first, _ := m.Allocate(2)
	_ = m.Free(first)

	second, err := m.Allocate(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 {
		t.Fatalf("got %d, want 2", len(second))
	}
	seen := map[BlockID]bool{}
	for _, id := range second {
		if int(id) >= 2 {
			t.Fatalf("reused id out of range: %d", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id: %d", id)
		}
		seen[id] = true
	}
}

func TestPropertyRefcounts(t *testing.T) {
	const numBlocks = 32
	const iterations = 1000

	rng := rand.New(rand.NewPCG(0xCAFEBABE, 0xDEADBEEF))

	m := mustNewManager(t, 16, numBlocks)
	truth := make([]int, numBlocks)
	var allocated []BlockID

	checkInvariants := func(t *testing.T, op string) {
		t.Helper()
		freeCount := 0
		used := 0
		for i := 0; i < numBlocks; i++ {
			if rc := m.Refcount(BlockID(i)); rc != truth[i] {
				t.Fatalf("%s: block %d refcount mismatch: manager=%d ground=%d", op, i, rc, truth[i])
			}
			if truth[i] == 0 {
				freeCount++
			} else {
				used++
			}
		}
		if m.FreeCount() != freeCount {
			t.Fatalf("%s: FreeCount mismatch: manager=%d ground=%d", op, m.FreeCount(), freeCount)
		}
		if m.UsedCount() != used {
			t.Fatalf("%s: UsedCount mismatch: manager=%d ground=%d", op, m.UsedCount(), used)
		}
	}

	for it := 0; it < iterations; it++ {
		switch rng.IntN(4) {
		case 0:
			n := rng.IntN(8) + 1
			ids, err := m.Allocate(n)
			if err != nil {
				if errors.Is(err, ErrCacheFull) {
					continue
				}
				t.Fatalf("iter %d Allocate(%d): %v", it, n, err)
			}
			for _, id := range ids {
				if truth[id] != 0 {
					t.Fatalf("iter %d: allocated block %d already had refcount %d", it, id, truth[id])
				}
				truth[id] = 1
				allocated = append(allocated, id)
			}
			checkInvariants(t, fmt.Sprintf("iter %d Allocate(%d)", it, n))
		case 1:
			if len(allocated) == 0 {
				continue
			}
			idx := rng.IntN(len(allocated))
			id := allocated[idx]
			if err := m.Ref(id); err != nil {
				t.Fatalf("iter %d Ref(%d): %v", it, id, err)
			}
			truth[id]++
			allocated = append(allocated, id)
			checkInvariants(t, fmt.Sprintf("iter %d Ref(%d)", it, id))
		case 2, 3:
			if len(allocated) == 0 {
				continue
			}
			idx := rng.IntN(len(allocated))
			id := allocated[idx]
			if err := m.Unref(id); err != nil {
				t.Fatalf("iter %d Unref(%d): %v", it, id, err)
			}
			truth[id]--
			allocated = append(allocated[:idx], allocated[idx+1:]...)
			checkInvariants(t, fmt.Sprintf("iter %d Unref(%d)", it, id))
		}
	}
}

func TestConcurrentAllocateFree(t *testing.T) {
	const numBlocks = 64
	m := mustNewManager(t, 16, numBlocks)

	const workers = 8
	const opsPerWorker = 200

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		seed := uint64(w + 1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(seed, seed*7919))
			var owned []BlockID
			for i := 0; i < opsPerWorker; i++ {
				if rng.IntN(2) == 0 {
					n := rng.IntN(3) + 1
					ids, err := m.Allocate(n)
					if errors.Is(err, ErrCacheFull) {
						continue
					}
					if err != nil {
						t.Errorf("Allocate: %v", err)
						return
					}
					owned = append(owned, ids...)
				} else if len(owned) > 0 {
					idx := rng.IntN(len(owned))
					id := owned[idx]
					if err := m.Unref(id); err != nil {
						t.Errorf("Unref: %v", err)
						return
					}
					owned = append(owned[:idx], owned[idx+1:]...)
				}
			}
			if len(owned) > 0 {
				_ = m.Free(owned)
			}
		}()
	}
	wg.Wait()

	if m.UsedCount() != 0 {
		t.Fatalf("UsedCount after all workers done: %d, want 0", m.UsedCount())
	}
}
