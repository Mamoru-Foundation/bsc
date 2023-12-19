package mamoru

import (
	"math/big"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mamoru/stats"
	"github.com/ethereum/go-ethereum/params"
)

var _ Feeder = &EthFeed{}

type EthFeed struct {
	chainConfig *params.ChainConfig
	stats       stats.Stats
}

func NewFeed(chainConfig *params.ChainConfig, statistic stats.Stats) Feeder {
	return &EthFeed{chainConfig: chainConfig, stats: statistic}
}

func (f *EthFeed) FeedBlock(block *types.Block) mamoru_sniffer.Block {
	var blockData mamoru_sniffer.Block
	blockData.BlockIndex = block.NumberU64()
	blockData.Hash = block.Hash().String()
	blockData.ParentHash = block.ParentHash().String()
	blockData.StateRoot = block.Root().String()
	blockData.Nonce = block.Nonce()
	blockData.Status = ""
	blockData.Timestamp = block.Time()
	blockData.BlockReward = block.Coinbase().Bytes()
	blockData.FeeRecipient = block.ReceiptHash().String()
	blockData.TotalDifficulty = block.Difficulty().Uint64()
	blockData.Size = float64(block.Size())
	blockData.GasUsed = block.GasUsed()
	blockData.GasLimit = block.GasLimit()

	f.stats.MarkBlocks()

	return blockData
}

func (f *EthFeed) FeedTransactions(blockNumber *big.Int, blockTime uint64, txs types.Transactions, receipts types.Receipts) []mamoru_sniffer.Transaction {
	signer := types.MakeSigner(f.chainConfig, blockNumber, blockTime)
	var transactions []mamoru_sniffer.Transaction

	for i, tx := range txs {
		var transaction mamoru_sniffer.Transaction
		transaction.TxIndex = uint32(i)
		transaction.TxHash = tx.Hash().String()
		transaction.Type = tx.Type()
		transaction.Nonce = tx.Nonce()
		if receipts.Len() > i {
			transaction.Status = receipts[i].Status
		}
		transaction.BlockIndex = blockNumber.Uint64()
		address, err := types.Sender(signer, tx)
		if err != nil {
			log.Error("Sender error", "err", err, "mamoru-tracer", "bsc_feed")
		}
		transaction.From = address.String()
		if tx.To() != nil {
			transaction.To = tx.To().String()
		}
		transaction.Value = tx.Value().Uint64()
		transaction.Fee = tx.GasFeeCap().Uint64()
		transaction.GasPrice = tx.GasPrice().Uint64()
		transaction.GasLimit = tx.Gas()
		transaction.GasUsed = tx.Cost().Uint64()
		transaction.Size = float64(tx.Size())
		transaction.Input = tx.Data()

		transactions = append(transactions, transaction)
	}

	f.stats.MarkTxs(uint64(len(transactions)))

	return transactions
}

func (f *EthFeed) FeedCallTraces(callFrames []*CallFrame, blockNumber uint64) []mamoru_sniffer.CallTrace {
	var callTraces []mamoru_sniffer.CallTrace
	for _, frame := range callFrames {
		var callTrace mamoru_sniffer.CallTrace
		callTrace.TxIndex = frame.TxIndex
		callTrace.BlockIndex = blockNumber
		callTrace.Type = frame.Type
		callTrace.To = frame.To
		callTrace.From = frame.From
		callTrace.Value = frame.Value
		callTrace.GasLimit = frame.Gas
		callTrace.GasUsed = frame.GasUsed
		callTrace.Input = frame.Input
		callTrace.Depth = frame.Depth

		callTraces = append(callTraces, callTrace)
	}

	f.stats.MarkCallTraces(uint64(len(callTraces)))

	return callTraces
}

func (f *EthFeed) FeedEvents(receipts types.Receipts) []mamoru_sniffer.Event {
	var events []mamoru_sniffer.Event
	for _, receipt := range receipts {
		for _, rlog := range receipt.Logs {
			var event mamoru_sniffer.Event
			event.Index = uint32(rlog.Index)
			event.BlockNumber = rlog.BlockNumber
			event.BlockHash = rlog.BlockHash.String()
			event.TxIndex = uint32(rlog.TxIndex)
			event.TxHash = rlog.TxHash.String()
			event.Address = rlog.Address.String()
			event.Data = rlog.Data
			for i, topic := range rlog.Topics {
				switch i {
				case 0:
					event.Topic0 = topic.Bytes()
				case 1:
					event.Topic1 = topic.Bytes()
				case 2:
					event.Topic2 = topic.Bytes()
				case 3:
					event.Topic3 = topic.Bytes()
				case 4:
					event.Topic4 = topic.Bytes()
				}
			}
			events = append(events, event)
		}
	}

	f.stats.MarkEvents(uint64(len(events)))

	return events
}

func (f *EthFeed) Stats() stats.Stats {
	return f.stats
}
