package navigator

type sortByPinID []*Pin

func (a sortByPinID) Len() int           { return len(a) }
func (a sortByPinID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByPinID) Less(i, j int) bool { return a[i].Hub.ID < a[j].Hub.ID }

type sortByLowestMeasuredCost []*Pin

func (a sortByLowestMeasuredCost) Len() int      { return len(a) }
func (a sortByLowestMeasuredCost) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByLowestMeasuredCost) Less(i, j int) bool {
	return a[i].measurements.GetCalculatedCost() < a[j].measurements.GetCalculatedCost()
}

type sortBySuggestedHopDistanceAndLowestMeasuredCost []*Pin

func (a sortBySuggestedHopDistanceAndLowestMeasuredCost) Len() int      { return len(a) }
func (a sortBySuggestedHopDistanceAndLowestMeasuredCost) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortBySuggestedHopDistanceAndLowestMeasuredCost) Less(i, j int) bool {
	// First sort by suggested hop distance.
	if a[i].analysis.SuggestedHopDistance != a[j].analysis.SuggestedHopDistance {
		return a[i].analysis.SuggestedHopDistance > a[j].analysis.SuggestedHopDistance
	}
	// Then by cost.
	return a[i].measurements.GetCalculatedCost() < a[j].measurements.GetCalculatedCost()
}

type sortByLowestMeasuredLatency []*Pin

func (a sortByLowestMeasuredLatency) Len() int      { return len(a) }
func (a sortByLowestMeasuredLatency) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByLowestMeasuredLatency) Less(i, j int) bool {
	x, _ := a[i].measurements.GetLatency()
	y, _ := a[j].measurements.GetLatency()
	return x < y
}

type sortByHighestMeasuredCapacity []*Pin

func (a sortByHighestMeasuredCapacity) Len() int      { return len(a) }
func (a sortByHighestMeasuredCapacity) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByHighestMeasuredCapacity) Less(i, j int) bool {
	x, _ := a[i].measurements.GetCapacity()
	y, _ := a[j].measurements.GetCapacity()
	return x > y
}
