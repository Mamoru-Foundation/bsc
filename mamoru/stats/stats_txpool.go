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
	// Processed Transactions number
	txs uint64
	// Processed Events number
	events uint64
	// Processed Call Traces number
	traces uint64

	mx sync.RWMutex
}

func NewStatsTxpool() *StatsTxpool {
	return &StatsTxpool{}
}

func (s *StatsTxpool) GetBlocks() uint64 {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return 0
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

func (s *StatsTxpool) MarkBlocks() {
	if blocksMetricTx != nil {
		blocksMetricTx.Mark(0)
	}
}

func (s *StatsTxpool) MarkTxs(txs uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.txs = txs
	if txsMetricTx != nil {
		txsMetricTx.Mark(int64(s.txs))
	}
}

func (s *StatsTxpool) MarkEvents(events uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.events = events
	if eventsMetricTx != nil {
		eventsMetricTx.Mark(int64(s.events))
	}
}

func (s *StatsTxpool) MarkCallTraces(traces uint64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.traces = traces
	if tracesMetricTx != nil {
		tracesMetricTx.Mark(int64(s.traces))
	}
}

func (s *StatsTxpool) Reset() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.txs = 0
	s.events = 0
	s.traces = 0
}
