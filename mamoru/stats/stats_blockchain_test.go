package stats

import (
	"reflect"
	"sync"
	"testing"
)

func TestAcumulatorStats(t *testing.T) {
	workers := 96
	steps := 666
	wont := &StatsBlockchain{
		blocks: uint64(1),
		txs:    uint64(2),
		events: uint64(2),
		traces: uint64(4),
	}
	t.Run("OK - stats", func(t *testing.T) {
		s := NewStatsBlockchain()
		wg := &sync.WaitGroup{}
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func(innerStat *StatsBlockchain) {
				defer wg.Done()
				for i := 0; i < steps; i++ {
					innerStat.MarkBlocks()
					innerStat.MarkCallTraces(4)
					innerStat.MarkTxs(2)
					innerStat.MarkEvents(2)
				}
			}(s) // Use Snapshot() to get a pointer to the same StatsBlockchain instance
		}
		wg.Wait()

		// Compare the entire structs
		if !reflect.DeepEqual(s, wont) {
			t.Errorf("StatsBlockchain = %v, want %v", s, wont)
		}
	})
}

func TestDeltaStats(t *testing.T) {
	workers := 96
	steps := 666
	wont := &StatsBlockchain{
		blocks: uint64(1),
		txs:    uint64(2),
		events: uint64(2),
		traces: uint64(4),
	}
	t.Run("OK - stats", func(t *testing.T) {
		s := NewStatsBlockchain()
		wg := &sync.WaitGroup{}
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func(innerStat *StatsBlockchain) {
				defer wg.Done()
				for i := 0; i < steps; i++ {
					innerStat.MarkBlocks()
					innerStat.MarkCallTraces(4)
					innerStat.MarkTxs(2)
					innerStat.MarkEvents(2)
				}
			}(s) // Use Snapshot() to get a pointer to the same StatsBlockchain instance
		}
		wg.Wait()

		// Compare individual counters
		if got, want := s.GetBlocks(), wont.GetBlocks(); got != want {
			t.Errorf("StatsBlockchain.GetBlocks() = %v, want %v", got, want)
		}
		if got, want := s.GetTxs(), wont.GetTxs(); got != want {
			t.Errorf("StatsBlockchain.GetTxs() = %v, want %v", got, want)
		}
		if got, want := s.GetEvents(), wont.GetEvents(); got != want {
			t.Errorf("StatsBlockchain.GetEvents() = %v, want %v", got, want)
		}
		if got, want := s.GetTraces(), wont.GetTraces(); got != want {
			t.Errorf("StatsBlockchain.GetTraces() = %v, want %v", got, want)
		}
	})
}
