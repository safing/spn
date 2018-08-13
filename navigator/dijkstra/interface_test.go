package dijkstra

func BuildTestNet() map[string]*TestElement {
	collection := make(map[string]*TestElement)

	// start
	e1 := NewTestElement(1000, false)
	collection[string(e1.id)] = e1

	e2 := NewTestElement(1000, false)
	collection[string(e2.id)] = e2

	e3 := NewTestElement(1000, false)
	collection[string(e3.id)] = e3

	e4 := NewTestElement(1000, false)
	collection[string(e4.id)] = e4

	// target
	e5 := NewTestElement(1000, true)
	collection[string(e5.id)] = e5

	// route 1
	e1.ConnectTo(e2)
	e2.ConnectTo(e5)

	// route 2
	e1.ConnectTo(e3)
	e3.ConnectTo(e4)
	e4.ConnectTo(e5)

	// interconnection
	e4.ConnectTo(e2)

	return collection
}

var (
	idCnt uint8
)

type TestElement struct {
	id        []byte
	neighbors []*TestElementConnection
	Cost      int
	IsTarget  bool
}

func NewTestElement(cost int, isTarget bool) *TestElement {
	idCnt++
	return &TestElement{
		id:       []byte{idCnt},
		Cost:     cost,
		IsTarget: isTarget,
	}
}

func (e *TestElement) ID() []byte {
	return e.id
}

func (e *TestElement) Neighbors() []Neighbor {
	return e.neighbors
}

func (e *TestElement) Cost() int {
	return e.Cost
}

func (e *TestElement) IsTarget() bool {
	return e.IsTarget
}

func (e *TestElement) ConnectTo(other *TestElement) {
	e.neighbors = append(e.neighbors, other)
	other.neighbors = append(other.neighbors, e)
}

type TestElementConnection struct {
	Element  *TestElement
	PathCost int
}

func (ec *TestElementConnection) Element() *TestElement {
	return ec.Element
}

func (ec *TestElementConnection) PathCost() int {
	return ec.PathCost
}
