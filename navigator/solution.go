package navigator

/*
type Solution struct {
	Cost      int
	Current   *Port
	Path      []*Port
	Processed bool
}

func NewSolutionFromDestination(port *Port) *Solution {
	return &Solution{
		Current: port,
		Cost:    port.Cost(),
		Path:    []*Port{port},
	}
}

func NewSolution(port *Port, cost int) *Solution {
	return &Solution{
		Current: port,
		Cost:    cost,
		Path:    []*Port{port},
	}
}

func (s *Solution) AddToPath(port *Port) {
	s.Path = append(s.Path, port)
}

func (s *Solution) CalculateCost(route *Route) int {
	return sanityCheckCost(s.Cost + route.Cost + route.Port.Cost())
}

func (s *Solution) CalculateConnectCost(route *Route) int {
	return sanityCheckCost(s.Cost + route.Cost + route.Port.ActiveRouteCost())
}

func (s *Solution) TakeCourse(route *Route, cost int) *Solution {
	newPath := make([]*Port, len(s.Path)+1)
	copy(newPath, s.Path)
	newPath[len(s.Path)] = route.Port

	return &Solution{
		Cost:    cost,
		Current: route.Port,
		Path:    newPath,
	}
}

func (s *Solution) Export() []*Port {
	path := make([]*Port, len(s.Path))
	copy(path, s.Path)
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
*/
