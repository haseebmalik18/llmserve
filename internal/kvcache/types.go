package kvcache

import "errors"

type BlockID uint32

type SeqID uint64

const InvalidBlock BlockID = ^BlockID(0)

var (
	ErrCacheFull         = errors.New("kvcache: not enough free blocks")
	ErrInvalidBlockID    = errors.New("kvcache: invalid block id")
	ErrRefcountUnderflow = errors.New("kvcache: refcount would go below zero")
	ErrInvalidCount      = errors.New("kvcache: invalid count")
)

type ManagerOptions struct {
	BlockSize int
	NumBlocks int
}

func DefaultManagerOptions() ManagerOptions {
	return ManagerOptions{BlockSize: 16, NumBlocks: 1024}
}
