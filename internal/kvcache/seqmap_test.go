package kvcache

import (
	"reflect"
	"testing"
)

func TestSeqMapGetMissing(t *testing.T) {
	s := NewSeqMap()
	if got := s.Get(SeqID(42)); got != nil {
		t.Fatalf("Get on missing: got %v, want nil", got)
	}
	if s.Has(SeqID(42)) {
		t.Fatalf("Has on missing: true")
	}
	if s.Len() != 0 {
		t.Fatalf("Len: got %d, want 0", s.Len())
	}
}

func TestSeqMapSetGet(t *testing.T) {
	s := NewSeqMap()
	blocks := []BlockID{1, 2, 3}
	s.Set(SeqID(7), blocks)

	got := s.Get(SeqID(7))
	if !reflect.DeepEqual(got, blocks) {
		t.Fatalf("got %v, want %v", got, blocks)
	}

	got[0] = 99
	if again := s.Get(SeqID(7)); again[0] != 1 {
		t.Fatalf("Get returned shared slice; mutation leaked")
	}
}

func TestSeqMapSetCopiesInput(t *testing.T) {
	s := NewSeqMap()
	blocks := []BlockID{1, 2, 3}
	s.Set(SeqID(7), blocks)
	blocks[0] = 99
	got := s.Get(SeqID(7))
	if got[0] != 1 {
		t.Fatalf("Set kept reference to input slice")
	}
}

func TestSeqMapAppend(t *testing.T) {
	s := NewSeqMap()
	s.Set(SeqID(1), []BlockID{1, 2})
	s.Append(SeqID(1), []BlockID{3, 4})
	got := s.Get(SeqID(1))
	want := []BlockID{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSeqMapAppendToNew(t *testing.T) {
	s := NewSeqMap()
	s.Append(SeqID(1), []BlockID{1, 2, 3})
	got := s.Get(SeqID(1))
	want := []BlockID{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSeqMapRemove(t *testing.T) {
	s := NewSeqMap()
	s.Set(SeqID(1), []BlockID{1, 2, 3})
	got := s.Remove(SeqID(1))
	if !reflect.DeepEqual(got, []BlockID{1, 2, 3}) {
		t.Fatalf("Remove returned %v", got)
	}
	if s.Has(SeqID(1)) {
		t.Fatalf("Has after Remove: true")
	}
}

func TestSeqMapRemoveMissing(t *testing.T) {
	s := NewSeqMap()
	got := s.Remove(SeqID(99))
	if len(got) != 0 {
		t.Fatalf("Remove missing: got %v", got)
	}
}

func TestSeqMapLen(t *testing.T) {
	s := NewSeqMap()
	s.Set(1, []BlockID{1})
	s.Set(2, []BlockID{2})
	s.Set(3, []BlockID{3})
	if s.Len() != 3 {
		t.Fatalf("Len: got %d", s.Len())
	}
	s.Remove(2)
	if s.Len() != 2 {
		t.Fatalf("Len after Remove: got %d", s.Len())
	}
}
