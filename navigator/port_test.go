package navigator

import (
	"testing"
)

func TestCostCalculation(t *testing.T) {

	port := &Port{
		Load: 0,
	}

	// test normal cost
	t.Logf("growth rate: %f", growthRate)

	if port.Cost() != int(costAt0) {
		t.Fatalf("Port cost at load 0 is %d, should be %f", port.Cost(), costAt0)
	}

	port.Load = 91
	if port.Cost() < int(costAt90) {
		t.Fatalf("Port cost at load 91 is %d, should be greater than %f", port.Cost(), costAt90)
	}
	if port.Cost() > int(costAt90*2) {
		t.Fatalf("Port cost at load 91 is %d, should be less than %f", port.Cost(), costAt90*2)
	}

	lastCost := 0
	for i := 0; i <= 100; i++ {
		port.Load = i
		cost := port.Cost()
		t.Logf("cost at load %d: %d\n", i, cost)
		if cost < lastCost {
			t.Fatalf("Port cost calcuation declined.")
		}
		lastCost = cost
	}

	// test cost with active connection

	t.Logf("active growth rate: %f", activeGrowthRate)

	port.Load = 0
	if port.ActiveCost() != int(activeCostAt0) {
		t.Fatalf("Port cost (active) at load 0 is %d, should be %f", port.ActiveCost(), activeCostAt0)
	}

	port.Load = 96
	if port.ActiveCost() < int(activeCostAt95) {
		t.Fatalf("Port cost (active) at load 96 is %d, should be greater than %f", port.ActiveCost(), activeCostAt95)
	}
	if port.ActiveCost() > int(activeCostAt95*2) {
		t.Fatalf("Port cost at load 96 is %d, should be less than %f", port.ActiveCost(), activeCostAt95*2)
	}

	lastCost = 0
	for i := 0; i <= 100; i++ {
		port.Load = i
		cost := port.ActiveCost()
		t.Logf("activeCost at load %d: %d\n", i, cost)
		if cost < lastCost {
			t.Fatalf("Port cost calcuation (active) declined.")
		}
		lastCost = cost
	}

}
