package unit

import (
	"context"
	"fmt"
	"time"
)

func resetScheduler() {
	currentUnitID.Store(0)
	finished.Store(0)
	clearanceUpTo.Store(minSlotPace)

	slotPace.Store(minSlotPace)
	pausedUnits.Store(0)
	highPrioUnits.Store(0)
}

func startTestScheduler() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	fmt.Println("========== STARTING TEST SCHEDULER ==========")

	go func() {
		err := slotScheduler(ctx)
		if err != nil {
			panic(err)
		}
	}()

	return func() {
		cancel()
		time.Sleep(slotDuration * 2)
		resetScheduler()
	}
}
