//go:build !llamacpp

package model

func initBackend() error { return ErrCGONotAvailable }
func freeBackend()        {}

func loadModelImpl(path string, opts ModelOptions) (modelImpl, error) {
	return nil, ErrCGONotAvailable
}
