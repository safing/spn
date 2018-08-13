package dijkstra

type Element interface {
	ID() []byte
	Neighbors() []Neighbor
	Cost() int
	IsTarget() bool
}

type Neighbor interface {
	Element() Element
	PathCost() int
}
