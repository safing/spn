package navigator

import (
	"fmt"
	"sort"
	"strings"
)

type Routes struct {
	All       []*Route
	maxCost   int // automatic
	maxRoutes int // manual setting
}

// Len is the number of elements in the collection.
func (r *Routes) Len() int {
	return len(r.All)
}

// Less reports whether the element with index i should sort before the element
// with index j.
func (r *Routes) Less(i, j int) bool {
	return r.All[i].TotalCost < r.All[j].TotalCost
}

// Swap swaps the elements with indexes i and j.
func (r *Routes) Swap(i, j int) {
	r.All[i], r.All[j] = r.All[j], r.All[i]
}

// isGoodEnough reports whether the route would survive a clean process.
func (r *Routes) isGoodEnough(route *Route) bool {
	if r.maxCost > 0 && route.TotalCost > r.maxCost {
		return false
	}
	return true
}

// add adds a Route if it is good enough.
func (r *Routes) add(route *Route) {
	if !r.isGoodEnough(route) {
		return
	}
	r.All = append(r.All, route.copy())
	r.clean()
}

// clean sort and shortens the list to the configured maximum.
func (r *Routes) clean() {
	// Sort Routes so that the best ones are on top.
	sort.Sort(r)
	// Remove all remaining from the list.
	if len(r.All) > r.maxRoutes {
		r.All = r.All[:r.maxRoutes]
	}
	// Set new maximum total cost.
	if len(r.All) >= r.maxRoutes {
		r.maxCost = r.All[len(r.All)-1].TotalCost
	}
}

type Route struct {
	// DstCost is the calculated cost between the Destination Hub and the destination IP.
	DstCost int

	// Path is a list of Transit Hubs and the Destination Hub, including the Cost
	// for each Hop.
	Path []*Hop

	// TotalCost is the sum of all costs of this Route.
	TotalCost int

	// Algorithm is the ID of the algorithm used to calculate the route.
	Algorithm string
}

type Hop struct {
	pin *Pin

	// Hub is the Hub ID.
	Hub string

	// Cost is the cost for both Lane to this Hub and the Hub itself.
	Cost int
}

// addHop adds a hop to the route.
func (r *Route) addHop(pin *Pin, cost int) {
	r.Path = append(r.Path, &Hop{
		pin:  pin,
		Cost: cost,
	})
	r.recalculateTotalCost()
}

// completeRoute completes the route by adding the destination cost of the
// connection between the last hop and the destination IP.
func (r *Route) completeRoute(dstCost int) {
	r.DstCost = dstCost
	r.recalculateTotalCost()
}

// removeHop removes the last hop from the Route.
func (r *Route) removeHop() {
	// Reset DstCost, as the route might have been completed.
	r.DstCost = 0

	if len(r.Path) >= 1 {
		r.Path = r.Path[:len(r.Path)-1]
	}
	r.recalculateTotalCost()
}

// recalculateTotalCost recalculates to total cost of this route.
func (r *Route) recalculateTotalCost() {
	r.TotalCost = r.DstCost
	for _, hop := range r.Path {
		if hop.pin.ActiveAPI != nil {
			// If we have an active connection, only take 90% of the cost.
			r.TotalCost += (hop.Cost * 9) / 10
		} else {
			r.TotalCost += hop.Cost
		}
	}
}

// copy makes a somewhat deep copy of the Route and returns it. Hops themselves
// are not copied, because their data does not change. Therefore, returned Hops
// may not be edited.
func (r *Route) copy() *Route {
	newRoute := &Route{
		Path:      make([]*Hop, len(r.Path)),
		TotalCost: r.TotalCost,
	}
	copy(newRoute.Path, r.Path)
	return newRoute
}

// makeExportReady fills in all the missing data fields which are meant for
// exporting only.
func (r *Routes) makeExportReady(algorithm string) {
	for _, route := range r.All {
		route.makeExportReady(algorithm)
	}
}

// makeExportReady fills in all the missing data fields which are meant for
// exporting only.
func (r *Route) makeExportReady(algorithm string) {
	r.Algorithm = algorithm
	for _, hop := range r.Path {
		hop.makeExportReady()
	}
}

// makeExportReady fills in all the missing data fields which are meant for
// exporting only.
func (hop *Hop) makeExportReady() {
	hop.Hub = hop.pin.Hub.ID
}

func (r *Route) String() string {
	s := make([]string, 0, len(r.Path)+1)
	for _, hop := range r.Path {
		s = append(s, fmt.Sprintf("=> %d$ %s", hop.Cost, hop.pin))
	}
	s = append(s, fmt.Sprintf("=> %d$", r.DstCost))
	return strings.Join(s, " ")
}
