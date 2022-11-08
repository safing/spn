package unit

import (
	"testing"
)

func BenchmarkScheduler(b *testing.B) {
	workers := 10

	// Start scheduler for test.
	cancel := startTestScheduler()
	defer cancel()

	// Init control structures.
	done := make(chan struct{})
	finishedCh := make(chan struct{})

	// Start workers.
	for i := 0; i < workers; i++ {
		go func() {
			for {
				u := New(nil)
				u.Start()
				u.Finish()
				select {
				case finishedCh <- struct{}{}:
				case <-done:
					return
				}
			}
		}()
	}

	// Start benchmark.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-finishedCh
	}
	b.StopTimer()

	// Cleanup.
	close(done)
}
