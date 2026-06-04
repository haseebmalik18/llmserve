//go:build llamacpp

package model

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/llama.cpp/include -I${SRCDIR}/../../third_party/llama.cpp/ggml/include
#cgo CFLAGS: -I/Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk/usr/include/c++/v1
#cgo CXXFLAGS: -I/Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk/usr/include/c++/v1
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/llama.cpp/build/src
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/llama.cpp/build/ggml/src
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/ggml-metal
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/ggml-blas
#cgo LDFLAGS: -lllama -lggml -lggml-cpu -lggml-base -lggml-metal -lggml-blas
#cgo darwin LDFLAGS: -framework Foundation -framework Metal -framework MetalKit -framework MetalPerformanceShaders -framework Accelerate -framework CoreFoundation -lc++

#include <stdlib.h>
#include <string.h>
#include "llama.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func initBackend() error {
	C.llama_backend_init()
	return nil
}

func freeBackend() {
	C.llama_backend_free()
}

type cgoModel struct {
	ptr    *C.struct_llama_model
	vocab  *C.struct_llama_vocab
	vocabSize int
}

type cgoContext struct {
	ptr *C.struct_llama_context
	m   *cgoModel
	pos C.int
}

func loadModelImpl(path string, opts ModelOptions) (modelImpl, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	params := C.llama_model_default_params()
	params.use_mmap = C.bool(opts.UseMmap)

	ptr := C.llama_model_load_from_file(cpath, params)
	if ptr == nil {
		return nil, fmt.Errorf("%w: %s", ErrModelLoad, path)
	}
	vocab := C.llama_model_get_vocab(ptr)
	if vocab == nil {
		C.llama_model_free(ptr)
		return nil, fmt.Errorf("%w: vocab is nil", ErrModelLoad)
	}
	return &cgoModel{
		ptr:       ptr,
		vocab:     vocab,
		vocabSize: int(C.llama_vocab_n_tokens(vocab)),
	}, nil
}

func (m *cgoModel) free() {
	if m.ptr != nil {
		C.llama_model_free(m.ptr)
		m.ptr = nil
		m.vocab = nil
	}
}

func (m *cgoModel) nVocab() int { return m.vocabSize }

func (m *cgoModel) bosToken() TokenID {
	return TokenID(C.llama_vocab_bos(m.vocab))
}

func (m *cgoModel) eosToken() TokenID {
	return TokenID(C.llama_vocab_eos(m.vocab))
}

func (m *cgoModel) tokenize(text string, addBOS bool) ([]TokenID, error) {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	textLen := C.int(len(text))

	// Try generous buffer first; if it fails we'll grow.
	bufSize := len(text) + 64
	for {
		buf := make([]C.llama_token, bufSize)
		n := C.llama_tokenize(
			m.vocab,
			ctext,
			textLen,
			(*C.llama_token)(unsafe.Pointer(&buf[0])),
			C.int(bufSize),
			C.bool(addBOS),
			C.bool(false),
		)
		if n >= 0 {
			out := make([]TokenID, n)
			for i := 0; i < int(n); i++ {
				out[i] = TokenID(buf[i])
			}
			return out, nil
		}
		// Negative return = needed buffer size
		needed := int(-n)
		if needed <= bufSize {
			return nil, fmt.Errorf("%w: unexpected negative %d for buffer %d", ErrTokenize, n, bufSize)
		}
		bufSize = needed
	}
}

func (m *cgoModel) detokenizeOne(t TokenID) string {
	var buf [64]C.char
	n := C.llama_token_to_piece(
		m.vocab,
		C.llama_token(t),
		&buf[0],
		C.int(len(buf)),
		0,
		C.bool(false),
	)
	if n <= 0 {
		// Negative means buffer too small; expand and retry.
		if n < 0 {
			big := make([]C.char, -int(n)+1)
			n2 := C.llama_token_to_piece(
				m.vocab,
				C.llama_token(t),
				&big[0],
				C.int(len(big)),
				0,
				C.bool(false),
			)
			if n2 <= 0 {
				return ""
			}
			return C.GoStringN(&big[0], n2)
		}
		return ""
	}
	return C.GoStringN(&buf[0], n)
}

func (m *cgoModel) newContext(opts ContextOptions) (contextImpl, error) {
	params := C.llama_context_default_params()
	if opts.NCtx > 0 {
		params.n_ctx = C.uint(opts.NCtx)
	}
	if opts.NBatch > 0 {
		params.n_batch = C.uint(opts.NBatch)
	}

	ctx := C.llama_init_from_model(m.ptr, params)
	if ctx == nil {
		return nil, ErrContextInit
	}
	return &cgoContext{ptr: ctx, m: m}, nil
}

func (c *cgoContext) free() {
	if c.ptr != nil {
		C.llama_free(c.ptr)
		c.ptr = nil
	}
}

func (c *cgoContext) decode(tokens []TokenID) error {
	if len(tokens) == 0 {
		return nil
	}
	cTokens := make([]C.llama_token, len(tokens))
	for i, t := range tokens {
		cTokens[i] = C.llama_token(t)
	}
	batch := C.llama_batch_get_one(
		(*C.llama_token)(unsafe.Pointer(&cTokens[0])),
		C.int(len(cTokens)),
	)
	ret := C.llama_decode(c.ptr, batch)
	if ret != 0 {
		return fmt.Errorf("%w: code %d", ErrDecode, int(ret))
	}
	c.pos += C.int(len(tokens))
	return nil
}

func (c *cgoContext) logitsLast() []float32 {
	ptr := C.llama_get_logits_ith(c.ptr, C.int(-1))
	if ptr == nil {
		return nil
	}
	out := make([]float32, c.m.vocabSize)
	src := unsafe.Slice((*float32)(unsafe.Pointer(ptr)), c.m.vocabSize)
	copy(out, src)
	return out
}
