package stats

import (
	"sync"

	"github.com/ethereum/go-ethereum/metrics"
)

var (
	blocksMetricTx = metrics.NewRegisteredMeter("mamoru/metrics_txpool/blocks", nil)
	txsMetricTx    = metrics.NewRegisteredMeter("mamoru/metrics_txpool/txs", nil)
	eventsMetricTx = metrics.NewRegisteredMeter("mamoru/metrics_txpool/events", nil)
	tracesMetricTx = metrics.NewRegisteredMeter("mamoru/metrics_txpool/traces", nil)
)

type StatsTxpool struct {
	blocks uint64 // TODO: remove
	txs    uint64
	events uint64
	traces uint64

	mx sync.RWMutex
}

func NewStatsTxpool() *StatsTxpool {
	return &StatsTxpool{}
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

func (s *StatsTxpool) GetBlocks() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.blocks
}

func (s *StatsTxpool) GetTxs() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.txs
}

func (s *StatsTxpool) GetEvents() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.events
}

func (s *StatsTxpool) GetTraces() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.traces
}

func (s *StatsTxpool) IncrementBlocks() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.blocks += 1
	if blocksMetricTx != nil {
		blocksMetricTx.Mark(int64(s.blocks))
	}
}

func (s *StatsTxpool) AddedTxs(txs uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.txs += txs
	if txsMetricTx != nil {
		txsMetricTx.Mark(int64(s.txs))
	}
}

func (s *StatsTxpool) AddedEvents(events uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.events += events
	if eventsMetricTx != nil {
		eventsMetricTx.Mark(int64(s.events))
	}
}

func (s *StatsTxpool) AddedCallTraces(traces uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.traces += traces
	if tracesMetricTx != nil {
		tracesMetricTx.Mark(int64(s.traces))
	}
}
