package navigator

import (
	"fmt"
	"net"
	"sort"
)

type ProximityResult struct {
	IP        net.IP
	Port      *Port
	Proximity int
}

func (pr *ProximityResult) String() string {
	return fmt.Sprintf("<Proximity from %s to %s: %d>", pr.Port.Name(), pr.IP, pr.Proximity)
}

type ProximityCollection struct {
	All          []*ProximityResult
	MinProximity int
}

func NewProximityCollection(elements int) *ProximityCollection {
	return &ProximityCollection{
		All: make([]*ProximityResult, 0, elements*2),
	}
}

// Len is the number of elements in the collection.
func (pc *ProximityCollection) Len() int {
	return len(pc.All)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (pc *ProximityCollection) Less(i, j int) bool {
	if pc.All[j] == nil || pc.All[i] == nil {
		return false
	}
	return pc.All[i].Proximity > pc.All[j].Proximity
}

// Swap swaps the elements with indexes i and j.
func (pc *ProximityCollection) Swap(i, j int) {
	pc.All[i], pc.All[j] = pc.All[j], pc.All[i]
}

func (pc *ProximityCollection) Add(result *ProximityResult) {
	if result.Proximity >= pc.MinProximity {
		pc.All = append(pc.All, result)
		if result.Proximity-10 > pc.MinProximity {
			pc.MinProximity = result.Proximity - 10
		}
	}
}

func (pc *ProximityCollection) Clean() {
	sort.Sort(pc)
	for i := 0; i < len(pc.All); i++ {
		if pc.All[i].Proximity < pc.MinProximity {
			pc.All = pc.All[:i]
			break
		}
	}
}
