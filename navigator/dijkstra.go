package navigator

/*
import (
	"fmt"
)

func (m *Map) FindShortestPath(considerActiveRoutes bool, destinations ...*Pin) (path []*Pin, ok bool, err error) {

	m.solutions = make(map[string]*Solution)

	m.CollectionLock.Lock()
	defer m.CollectionLock.Unlock()

	var solutions []*Solution
	var solutionCandidate *Solution
	var roundsCandidateWon int

	// create destinations
	for _, destPort := range destinations {
		_, ok = m.Collection[string(destPort.Name())]
		if !ok {
			return nil, false, fmt.Errorf("port17/navigator: destination Port %s not in collection", destPort)
		}
		m.solutions[string(destPort.Name())] = NewSolutionFromDestination(destPort)

		if considerActiveRoutes && destPort.HasActiveRoute() {
			activeRouteCost := destPort.ActiveRouteCost()
			if activeRouteCost <= m.IgnoreAbove {
				if solutionCandidate == nil || activeRouteCost <= solutionCandidate.Cost {
					solutionCandidate = NewSolution(destPort, activeRouteCost)
				}
			}
		}
	}

	for {
		// get solutions to process
		solutions = m.getUnprocessedSolutions()
		if len(solutions) == 0 {
			if solutionCandidate == nil {
				return nil, false, nil
			}
			return solutionCandidate.Export(), true, nil
		}

		// evaluate solutions
		for _, solution := range solutions {
			betterCandidate := m.evaluateRoutes(solution, solutionCandidate, considerActiveRoutes)
			if betterCandidate != nil {
				solutionCandidate = betterCandidate
				roundsCandidateWon = 0
			}
		}
		roundsCandidateWon++

		// check if the solutionCandidate could sustain two times in a row
		if solutionCandidate != nil && roundsCandidateWon >= 3 {
			return solutionCandidate.Export(), true, nil
		}

	}

}

func (m *Map) evaluateRoutes(s *Solution, solutionCandidate *Solution, considerActiveRoutes bool) (betterCandidate *Solution) {
	betterCandidate = solutionCandidate
	// log.Debugf("evaluating %d", s.Current.ID()[0])

	for _, route := range s.Current.Routes {

		// first check normal routes
		competingSolution, ok := m.solutions[route.Port.Name()]
		hopWouldCost := s.CalculateCost(route)
		if (!ok || competingSolution.Cost > hopWouldCost) && hopWouldCost <= m.IgnoreAbove {
			// no solution yet
			// OR replace solution with better one
			// IF cost does not exceed max cost

			// log.Debugf("saving new possible solution: %d", route.Port.Name()[0])
			// create new solution
			new := s.TakeCourse(route, hopWouldCost)
			// save to solutions
			m.solutions[route.Port.Name()] = new
			// check if we reached the primary port, update betterCandidate
			if new.Current.Equal(m.PrimaryPort) {
				if betterCandidate == nil || new.Cost < betterCandidate.Cost {
					betterCandidate = new
					// log.Debugf("found new best candidate: %s @ %d", printPortPath(betterCandidate.Export()), betterCandidate.Cost)
				}
			}
		}

		// then, check if we can connect to an active route
		if considerActiveRoutes && route.Port.HasActiveRoute() {
			connectToActiveRouteCost := s.CalculateConnectCost(route)
			if connectToActiveRouteCost <= m.IgnoreAbove {
				if betterCandidate == nil || connectToActiveRouteCost <= betterCandidate.Cost {
					betterCandidate = s.TakeCourse(route, connectToActiveRouteCost)
					// log.Debugf("found new best candidate: %s @ %d", printPortPath(betterCandidate.Export()), betterCandidate.Cost)
				}
			}
		}

	}
	s.Processed = true
	return
}
*/
