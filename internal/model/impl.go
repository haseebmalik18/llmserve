package model

type modelImpl interface {
	free()
	nVocab() int
	bosToken() TokenID
	eosToken() TokenID
	tokenize(text string, addBOS bool) ([]TokenID, error)
	detokenizeOne(t TokenID) string
	newContext(opts ContextOptions) (contextImpl, error)
}

type contextImpl interface {
	free()
	decode(tokens []TokenID) error
	logitsLast() []float32
}
