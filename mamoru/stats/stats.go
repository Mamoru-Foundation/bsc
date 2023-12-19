package stats

type Stats interface {
	GetBlocks() uint64
	GetTxs() uint64
	GetEvents() uint64
	GetTraces() uint64
	MarkBlocks()
	MarkTxs(uint64)
	MarkEvents(uint64)
	MarkCallTraces(uint64)
	Reset()
}
