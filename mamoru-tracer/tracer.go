package mamoru_tracer

import (
	"context"
	"github.com/Mamoru-Foundation/mamoru-sniffer-go/evm_types"
	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mamoru-tracer/tracer"
	"math/big"
	"os"
	"time"
)

var sniffer *mamoru_sniffer.Sniffer

func init() {
	if IsSnifferEnable() {
		var err error
		sniffer, err = mamoru_sniffer.Connect()
		if err != nil {
			panic(err)
		}
	}
}

func Trace(ctx context.Context, tracerCfg *tracer.Config, block *types.Block, receipts types.Receipts, feeder tracer.Feeder) {
	builder := mamoru_sniffer.NewBlockchainDataCtxBuilder()
	snifferStart := time.Now()
	defer finish(snifferStart, builder, block.Number(), block.Hash())

	callFrames, err := tracer.TraceBlock(ctx, tracerCfg, block)
	if err != nil {
		return
	}
	log.Info("Trace block", "elapsed", common.PrettyDuration(time.Since(snifferStart)))

	blockData := feeder.FeedBlock(block)

	builder.AddData(evm_types.NewBlockData([]evm_types.Block{
		blockData,
	}))

	transactions, transactionArgs := feeder.FeedTransactions(block, receipts)

	builder.AddData(evm_types.NewTransactionArgData(
		transactionArgs,
	))
	builder.AddData(evm_types.NewTransactionData(
		transactions,
	))

	callTraces, callTraceArgs := feeder.FeedCalTraces(callFrames, block.NumberU64())

	builder.AddData(evm_types.NewCallTraceArgData(
		callTraceArgs,
	))
	builder.AddData(evm_types.NewCallTraceData(
		callTraces,
	))

	events, eventTopics := feeder.FeedEvents(receipts)

	builder.AddData(evm_types.NewEventTopicData(
		eventTopics,
	))
	builder.AddData(evm_types.NewEventData(
		events,
	))
}

func finish(start time.Time, builder mamoru_sniffer.BlockchainDataCtxBuilder, blockNumber *big.Int, blockHash common.Hash) {
	if sniffer != nil {
		sniffer.ObserveData(builder.Finish(blockNumber.String(), blockHash.String(), time.Now()))
	}
	logCtx := []interface{}{
		"elapsed", common.PrettyDuration(time.Since(start)),
		"number", blockNumber,
		"hash", blockHash,
	}
	log.Info("Sniff block finish", logCtx...)
}

func IsSnifferEnable() bool {
	isEnable, ok := os.LookupEnv("MAMORU_SNIFFER_ENABLE")
	return ok && isEnable == "true"
}
