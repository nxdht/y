package y

import "sync"

const (
	idMin = uint64(1)
	idMax = uint64(0xFFFFFFFFFFFFFFFF)
)

var (
	DefaultIDGenerator = NewUint64IDGenerator()
)

type IDGenerator interface {
	Next() interface{}
}

type Uint64IDGenerator struct {
	id    uint64
	mutex sync.Mutex
}

func (g *Uint64IDGenerator) Next() interface{} {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	x := g.id

	if g.id == idMax {
		g.id = idMin
	} else {
		g.id++
	}

	return x
}

func NewUint64IDGenerator() IDGenerator {
	return &Uint64IDGenerator{
		id: idMin,
	}
}
