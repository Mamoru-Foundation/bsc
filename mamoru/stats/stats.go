package stats

type Stats interface {
	//Stats() *Stats
	GetTxs() uint64
	GetEvents() uint64
	GetTraces() uint64
	IncrementBlocks()
	AddedTxs(uint64)
	AddedEvents(uint64)
	AddedCallTraces(uint64)
}
