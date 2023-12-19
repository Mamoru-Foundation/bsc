package mempool

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"

	tm "github.com/tendermint/tendermint/libs/common"

	"github.com/ethereum/go-ethereum/mamoru"
)

type blockChain interface {
	core.ChainContext
	CurrentBlock() *types.Header
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)
	State() (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription
}

type TxPoolBackendSniffer struct {
	txPool      BcTxPool
	chain       blockChain
	chainConfig *params.ChainConfig
	feeder      mamoru.Feeder

	newHeadEvent chan core.ChainHeadEvent
	newTxsEvent  chan core.NewTxsEvent

	chEv chan core.ChainEvent

	TxSub   event.Subscription
	headSub event.Subscription

	chEvSub event.Subscription

	ctx     context.Context
	mu      sync.RWMutex
	sniffer *mamoru.Sniffer
}

func NewTxPoolBackendSniffer(ctx context.Context, txPool BcTxPool, chain blockChain, chainConfig *params.ChainConfig, feeder mamoru.Feeder, mamoruSniffer *mamoru.Sniffer) *TxPoolBackendSniffer {
	if mamoruSniffer == nil {
		mamoruSniffer = mamoru.NewSniffer()
	}
	sb := &TxPoolBackendSniffer{
		txPool:      txPool,
		chain:       chain,
		chainConfig: chainConfig,

		newTxsEvent:  make(chan core.NewTxsEvent, 1024),
		newHeadEvent: make(chan core.ChainHeadEvent, 10),

		chEv: make(chan core.ChainEvent, 10),

		feeder: feeder,

		ctx: ctx,
		mu:  sync.RWMutex{},

		sniffer: mamoruSniffer,
	}
	sb.TxSub = sb.SubscribeNewTxsEvent(sb.newTxsEvent)
	sb.headSub = sb.SubscribeChainHeadEvent(sb.newHeadEvent)
	sb.chEvSub = sb.SubscribeChainEvent(sb.chEv)

	go sb.SnifferLoop()

	return sb
}

func (bc *TxPoolBackendSniffer) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return bc.txPool.SubscribeNewTxsEvent(ch)
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *TxPoolBackendSniffer) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chain.SubscribeChainHeadEvent(ch)
}

func (bc *TxPoolBackendSniffer) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return bc.chain.SubscribeChainEvent(ch)
}

func (bc *TxPoolBackendSniffer) SnifferLoop() {
	defer func() {
		bc.TxSub.Unsubscribe()
		bc.headSub.Unsubscribe()
		bc.chEvSub.Unsubscribe()
	}()

	ctx, cancel := context.WithCancel(bc.ctx)
	var block = bc.chain.CurrentBlock()

	for {
		select {
		case <-bc.ctx.Done():
		case <-bc.TxSub.Err():
		case <-bc.headSub.Err():
		case <-bc.chEvSub.Err():
			cancel()
			return

		case newTx := <-bc.newTxsEvent:
			go bc.process(ctx, block, newTx.Txs)

		case newHead := <-bc.newHeadEvent:
			if newHead.Block != nil && newHead.Block.NumberU64() > block.Number.Uint64() {
				log.Info("New core.ChainHeadEvent", "number", newHead.Block.NumberU64(), "ctx", mamoru.CtxTxpool)
				bc.mu.RLock()
				block = newHead.Block.Header()
				bc.mu.RUnlock()
			}

		case newChEv := <-bc.chEv:
			if newChEv.Block != nil && newChEv.Block.NumberU64() > block.Number.Uint64() {
				log.Info("New core.ChainEvent", "number", newChEv.Block.NumberU64(), "ctx", mamoru.CtxTxpool)
				bc.mu.RLock()
				block = newChEv.Block.Header()
				bc.mu.RUnlock()
			}
		}
	}
}

func (bc *TxPoolBackendSniffer) process(ctx context.Context, header *types.Header, txs types.Transactions) {
	if ctx.Err() != nil || !bc.sniffer.CheckRequirements() {
		return
	}
	blockNumber := header.Number.Uint64()
	log.Info("Mamoru start", "number", blockNumber, "txs", txs.Len(), "ctx", mamoru.CtxTxpool)
	startTime := time.Now()

	// Create tracer context
	tracer := mamoru.NewTracer(bc.feeder)

	// Set txpool context
	tracer.SetTxpoolCtx()

	var receipts types.Receipts
	var callTraces []*mamoru.CallFrame

	stateDb, err := bc.chain.StateAt(header.Root)
	if err != nil {
		log.Error("Mamoru State", "number", blockNumber, "err", err, "ctx", mamoru.CtxTxpool)
	}

	stateDb = stateDb.Copy()
	header.BaseFee = new(big.Int).SetUint64(0)

	for index, tx := range txs {
		chCtx := core.ChainContext(bc.chain)
		stateDb.SetTxContext(tx.Hash(), index)
		from, err := types.Sender(types.LatestSigner(bc.chainConfig), tx)
		if err != nil {
			log.Error("types.Sender", "number", blockNumber, "err", err, "ctx", mamoru.CtxTxpool)
		}

		if tx.Nonce() != stateDb.GetNonce(from) {
			stateDb.SetNonce(from, tx.Nonce())
		}

		callStackTracer := mamoru.NewCallStackTracer(types.Transactions{tx}, 1, tm.RandStr(8), false, mamoru.CtxTxpool)
		receipt, err := core.ApplyTransaction(bc.chainConfig, chCtx, &from, new(core.GasPool).AddGas(tx.Gas()), stateDb, header, tx,
			new(uint64), vm.Config{Tracer: callStackTracer, NoBaseFee: true})
		if err != nil {
			log.Error("Mamoru Tx Apply", "number", blockNumber, "err", err,
				"tx.hash", tx.Hash().String(), "ctx", mamoru.CtxTxpool)
			break
		}

		// Clean receipt
		cleanReceiptAndLogs(receipt)

		receipts = append(receipts, receipt)

		callFrames, err := callStackTracer.TakeResult()
		if err != nil {
			log.Error("Mamoru tracer result", "number", blockNumber, "err", err, "ctx", mamoru.CtxTxpool)
			break
		}

		var bytesLength int
		for i := 0; i < len(callFrames); i++ {
			bytesLength += len(callFrames[i].Input)
		}

		log.Info("Mamoru callFrames", "number", blockNumber, "callFrames", len(callFrames), "calls.input.len", bytesLength, "ctx", mamoru.CtxTxpool)
		callTraces = append(callTraces, callFrames...)
	}

	tracer.FeedTransactions(header.Number, header.Time, txs, receipts)
	tracer.FeedEvents(receipts)
	tracer.FeedCallTraces(callTraces, blockNumber)

	tracer.Send(startTime, header.Number, header.Hash(), mamoru.CtxTxpool)
}

func cleanReceiptAndLogs(receipt *types.Receipt) {
	receipt.BlockNumber = big.NewInt(0)
	receipt.BlockHash = common.Hash{}
	for _, l := range receipt.Logs {
		l.BlockNumber = 0
		l.BlockHash = common.Hash{}
	}
}
