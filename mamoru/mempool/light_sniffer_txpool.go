package mempool

import (
	"context"

	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mamoru"
	"github.com/ethereum/go-ethereum/mamoru/call_tracer"
	"github.com/ethereum/go-ethereum/params"
)

type lightBlockChain interface {
	core.ChainContext

	GetBlockByHash(context.Context, common.Hash) (*types.Block, error)
	CurrentHeader() *types.Header
	Odr() light.OdrBackend

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
}

type TxPoolLightSniffer struct {
	txPool      LightTxPool
	chain       lightBlockChain
	chainConfig *params.ChainConfig

	newHeadEvent chan core.ChainHeadEvent
	newTxsEvent  chan core.NewTxsEvent

	TxSub   event.Subscription
	headSub event.Subscription

	ctx context.Context

	sniffer *mamoru.Sniffer
}

func NewLightSniffer(ctx context.Context, txPool LightTxPool, chain lightBlockChain, chainConfig *params.ChainConfig, mamoruSniffer *mamoru.Sniffer) *TxPoolLightSniffer {
	if mamoruSniffer == nil {
		mamoruSniffer = mamoru.NewSniffer()
	}
	sb := &TxPoolLightSniffer{
		txPool:       txPool,
		chain:        chain,
		chainConfig:  chainConfig,
		newHeadEvent: make(chan core.ChainHeadEvent, 10),
		newTxsEvent:  make(chan core.NewTxsEvent, 1024),

		ctx:     ctx,
		sniffer: mamoruSniffer,
	}
	sb.headSub = sb.SubscribeChainHeadEvent(sb.newHeadEvent)
	sb.TxSub = sb.SubscribeNewTxsEvent(sb.newTxsEvent)

	go sb.SnifferLoop()

	return sb
}

func (bc *TxPoolLightSniffer) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return bc.txPool.SubscribeNewTxsEvent(ch)
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *TxPoolLightSniffer) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chain.SubscribeChainHeadEvent(ch)
}

func (bc *TxPoolLightSniffer) SnifferLoop() {
	ctx, cancel := context.WithCancel(bc.ctx)

	defer func() {
		bc.headSub.Unsubscribe()
		bc.TxSub.Unsubscribe()
	}()
	var head *types.Header = bc.chain.CurrentHeader()
	for {
		select {
		case <-bc.ctx.Done():
		case <-bc.headSub.Err():
		case <-bc.TxSub.Err():
			cancel()
			return

		case newHead := <-bc.newHeadEvent:
			if newHead.Block != nil {
				head = newHead.Block.Header()
				//go bc.processHead(ctx, newHead.Block.Header())
			}
		case txsEvent := <-bc.newTxsEvent:
			if txsEvent.Txs != nil {
				go bc.processTxs(ctx, txsEvent.Txs, head)
			}
		}
	}
}

func (bc *TxPoolLightSniffer) processTxs(ctx context.Context, txs types.Transactions, head *types.Header) {
	if ctx.Err() != nil {
		return
	}
	if !bc.sniffer.CheckRequirements() {
		return
	}

	log.Info("Mamoru TxPool Sniffer start", "txs", txs.Len(), "number", head.Number.Uint64(), "ctx", "lighttxpool")
	startTime := time.Now()

	// Create tracer context
	tracer := mamoru.NewTracer(mamoru.NewFeed(bc.chainConfig))

	// Set txpool context
	tracer.SetTxpoolCtx()

	var receipts types.Receipts
	parentBlock, err := bc.chain.GetBlockByHash(ctx, head.ParentHash)
	if err != nil {
		log.Error("Mamoru parent block", "number", head.Number.Uint64(), "err", err, "ctx", "lighttxpool")
		return
	}
	stateDb := light.NewState(ctx, parentBlock.Header(), bc.chain.Odr())

	for index, tx := range txs {
		calltracer, err := mamoru.NewCallTracer(false)
		if err != nil {
			log.Error("Mamoru Call tracer", "err", err, "ctx", "lighttxpool")
		}

		chCtx := core.ChainContext(bc.chain)
		author, _ := types.LatestSigner(bc.chainConfig).Sender(tx)
		gasPool := new(core.GasPool).AddGas(tx.Gas())

		var gasUsed = new(uint64)
		*gasUsed = head.GasUsed

		stateDb.Prepare(tx.Hash(), index)
		from, err := types.Sender(types.LatestSigner(bc.chainConfig), tx)
		if err != nil {
			log.Error("types.Sender", "err", err, "number", head.Number.Uint64(), "ctx", "lighttxpool")
		}
		if tx.Nonce() > stateDb.GetNonce(from) {
			stateDb.SetNonce(from, tx.Nonce())
		}
		log.Info("ApplyTransaction", "tx.hash", tx.Hash().String(), "tx.nonce", tx.Nonce(), "stNonce", stateDb.GetNonce(from), "number", head.Number.Uint64(), "ctx", "lighttxpool")

		receipt, err := core.ApplyTransaction(bc.chainConfig, chCtx, &author, gasPool, stateDb, head, tx,
			gasUsed, vm.Config{Debug: true, Tracer: calltracer, NoBaseFee: true})
		if err != nil {
			log.Error("Mamoru Apply Transaction", "err", err, "number", head.Number.Uint64(),
				"tx.hash", tx.Hash().String(), "ctx", "lighttxpool")
			break
		}

		// Clean receipt
		cleanReceiptAndLogs(receipt)

		receipts = append(receipts, receipt)

		callFrames, err := calltracer.GetResult()
		if err != nil {
			log.Error("Mamoru tracer result", "err", err, "number", head.Number.Uint64(),
				"ctx", "lighttxpool")
			break
		}

		tracer.FeedCalTraces(callFrames, head.Number.Uint64())
	}
	//currentBlock, err := bc.chain.GetBlockByHash(ctx, head.Hash())
	//if err != nil {
	//	log.Error("Mamoru block", "number", head.Number.Uint64(), "err", err, "ctx", "lighttxpool")
	//	return
	//}
	//tracer.FeedBlock(currentBlock)
	tracer.FeedTransactions(head.Number, txs, receipts)
	tracer.FeedEvents(receipts)
	tracer.Send(startTime, head.Number, head.Hash(), "lighttxpool")
}

func (bc *TxPoolLightSniffer) processHead(ctx context.Context, head *types.Header) {
	if ctx.Err() != nil {
		return
	}
	if !bc.sniffer.CheckRequirements() {
		return
	}

	log.Info("Mamoru LightTxPool Sniffer start", "number", head.Number.Uint64(), "ctx", "lighttxpool")
	startTime := time.Now()

	// Create tracer context
	tracer := mamoru.NewTracer(mamoru.NewFeed(bc.chainConfig))

	// Set txpool context
	tracer.SetTxpoolCtx()

	// Create call tracer
	stateDb := light.NewState(ctx, head, bc.chain.Odr())

	newBlock, err := bc.chain.GetBlockByHash(ctx, head.Hash())
	if err != nil {
		log.Error("Mamoru current block", "number", head.Number.Uint64(), "err", err, "ctx", "lighttxpool")
		return
	}
	tracer.FeedBlock(newBlock)

	callFrames, err := call_tracer.TraceBlock(ctx, call_tracer.NewTracerConfig(stateDb, bc.chainConfig, bc.chain), newBlock)
	if err != nil {
		log.Error("Mamoru block trace", "number", head.Number.Uint64(), "err", err, "ctx", "lighttxpool")
		return
	}

	for _, call := range callFrames {
		result := call.Result
		tracer.FeedCalTraces(result, head.Number.Uint64())
	}

	receipts, err := light.GetBlockReceipts(ctx, bc.chain.Odr(), newBlock.Hash(), newBlock.NumberU64())
	if err != nil {
		log.Error("Mamoru block receipt", "number", head.Number.Uint64(), "err", err, "ctx", "lighttxpool")
		return
	}

	tracer.FeedTransactions(newBlock.Number(), newBlock.Transactions(), receipts)
	tracer.FeedEvents(receipts)

	// finish tracer context
	tracer.Send(startTime, newBlock.Number(), newBlock.Hash(), "lighttxpool")
}
