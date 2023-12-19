package mamoru

import (
	"math/big"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/mamoru/stats"
)

type Feeder interface {
	FeedBlock(*types.Block) mamoru_sniffer.Block
	FeedTransactions(blockNumber *big.Int, blockTime uint64, txs types.Transactions, receipts types.Receipts) []mamoru_sniffer.Transaction
	FeedEvents(types.Receipts) []mamoru_sniffer.Event
	FeedCallTraces([]*CallFrame, uint64) []mamoru_sniffer.CallTrace
	Stats() stats.Stats
}
