package stats

import (
	"sync"

	"github.com/ethereum/go-ethereum/metrics"
)

var (
	blocksMetric = metrics.NewRegisteredMeter("mamoru/metrics/blocks", nil)
	txsMetric    = metrics.NewRegisteredMeter("mamoru/metrics/txs", nil)
	eventsMetric = metrics.NewRegisteredMeter("mamoru/metrics/events", nil)
	tracesMetric = metrics.NewRegisteredMeter("mamoru/metrics/traces", nil)
)

type StatsBlockchain struct {
	blocks uint64
	txs    uint64
	events uint64
	traces uint64

	mx sync.RWMutex
}

func NewStatsBlockchain() *StatsBlockchain {
	return &StatsBlockchain{}
}

//func (s *StatsBlockchain) Stats() *Stats {
//	s.mx.RLock()
//	defer s.mx.RUnlock()
//	return s
//}
//
//func (s *StatsBlockchain) Snapshot() Stats {
//	s.mx.RLock()
//	defer s.mx.RUnlock()
//	return &StatsBlockchain{
//		blocks: s.blocks,
//		txs:    s.txs,
//		events: s.events,
//		traces: s.traces,
//		mx:     sync.RWMutex{},
//	}
//}

func (s *StatsBlockchain) GetBlocks() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.blocks
}

func (s *StatsBlockchain) GetTxs() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.txs
}

func (s *StatsBlockchain) GetEvents() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.events
}

func (s *StatsBlockchain) GetTraces() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.traces
}

func (s *StatsBlockchain) IncrementBlocks() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.blocks += 1
	if blocksMetric != nil {
		blocksMetric.Mark(int64(s.blocks))
	}
}

func (s *StatsBlockchain) AddedTxs(txs uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.txs += txs
	if txsMetric != nil {
		txsMetric.Mark(int64(s.txs))
	}
}

func (s *StatsBlockchain) AddedEvents(events uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.events += events
	if eventsMetric != nil {
		eventsMetric.Mark(int64(s.events))
	}
}

func (s *StatsBlockchain) AddedCallTraces(traces uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.traces += traces
	if tracesMetric != nil {
		tracesMetric.Mark(int64(s.traces))
	}
}
