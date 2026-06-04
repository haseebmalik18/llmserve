package model

type Model struct {
	impl modelImpl
}

type Context struct {
	impl  contextImpl
	model *Model
}

func InitBackend() error {
	return initBackend()
}

func FreeBackend() {
	freeBackend()
}

func LoadModel(path string, opts ModelOptions) (*Model, error) {
	impl, err := loadModelImpl(path, opts)
	if err != nil {
		return nil, err
	}
	return &Model{impl: impl}, nil
}

func (m *Model) Free() {
	if m == nil {
		return
	}
	m.impl.free()
}

func (m *Model) NVocab() int {
	return m.impl.nVocab()
}

func (m *Model) BOSToken() TokenID {
	return m.impl.bosToken()
}

func (m *Model) EOSToken() TokenID {
	return m.impl.eosToken()
}

func (m *Model) Tokenize(text string, addBOS bool) ([]TokenID, error) {
	return m.impl.tokenize(text, addBOS)
}

func (m *Model) DetokenizeOne(t TokenID) string {
	return m.impl.detokenizeOne(t)
}

func (m *Model) NewContext(opts ContextOptions) (*Context, error) {
	impl, err := m.impl.newContext(opts)
	if err != nil {
		return nil, err
	}
	return &Context{impl: impl, model: m}, nil
}

func (c *Context) Free() {
	if c == nil {
		return
	}
	c.impl.free()
}

func (c *Context) Decode(tokens []TokenID) error {
	return c.impl.decode(tokens)
}

func (c *Context) LogitsLast() []float32 {
	return c.impl.logitsLast()
}

func SampleGreedy(logits []float32) TokenID {
	if len(logits) == 0 {
		return InvalidToken
	}
	bestIdx := 0
	best := logits[0]
	for i, v := range logits {
		if v > best {
			best = v
			bestIdx = i
		}
	}
	return TokenID(bestIdx)
}
