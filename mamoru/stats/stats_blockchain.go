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
	// Processed Blocks number
	blocks uint64
	// Processed Transactions number
	txs uint64
	// Processed Events number
	events uint64
	// Processed Call Traces number
	traces uint64

	mx sync.RWMutex
}

func NewStatsBlockchain() *StatsBlockchain {
	return &StatsBlockchain{}
}

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

func (s *StatsBlockchain) MarkBlocks() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.blocks = 1
	if blocksMetric != nil {
		blocksMetric.Mark(int64(s.blocks))
	}
}

func (s *StatsBlockchain) MarkTxs(txs uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.txs = txs
	if txsMetric != nil {
		txsMetric.Mark(int64(s.txs))
	}
}

func (s *StatsBlockchain) MarkEvents(events uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.events = events
	if eventsMetric != nil {
		eventsMetric.Mark(int64(s.events))
	}
}

func (s *StatsBlockchain) MarkCallTraces(traces uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.traces = traces
	if tracesMetric != nil {
		tracesMetric.Mark(int64(s.traces))
	}
}
