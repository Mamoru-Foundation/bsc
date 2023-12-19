package mempool

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum/mamoru"
	"github.com/ethereum/go-ethereum/mamoru/stats"
)

var (
	// testTxPoolConfig is a transaction pool configuration without stateful disk
	// sideeffects used during testing.
	testTxPoolConfig legacypool.Config

	// eip1559Config is a chain config with EIP-1559 enabled at block 0.
	eip1559Config   *params.ChainConfig
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)
)

func init() {
	testTxPoolConfig = legacypool.DefaultConfig
	testTxPoolConfig.Journal = ""

	cpy := *params.TestChainConfig
	eip1559Config = &cpy
	eip1559Config.BerlinBlock = common.Big0
	eip1559Config.LondonBlock = common.Big0
}

type testBlockChain struct {
	gasLimit           uint64 // must be first field for 64 bit alignment (atomic access)
	statedb            *state.StateDB
	chainHeadFeed      *event.Feed
	chainEventFeed     *event.Feed
	chainSideEventFeed *event.Feed
	engine             consensus.Engine
}

func (bc *testBlockChain) Config() *params.ChainConfig {
	return params.TestChainConfig
}

func (bc *testBlockChain) GetHeader(common.Hash, uint64) *types.Header {
	return &types.Header{}
}

func (bc *testBlockChain) State() (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return bc.chainEventFeed.Subscribe(ch)
}

func (bc *testBlockChain) CurrentBlock() *types.Header {
	return &types.Header{
		Number:   big.NewInt(1),
		GasLimit: atomic.LoadUint64(&bc.gasLimit),
	}
}

func (bc *testBlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return types.NewBlock(bc.CurrentBlock(), nil, nil, nil, trie.NewStackTrie(nil))
}

func (bc *testBlockChain) StateAt(common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func (bc *testBlockChain) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return bc.chainSideEventFeed.Subscribe(ch)
}

func (bc *testBlockChain) Engine() consensus.Engine {
	return bc.engine
}

func (bc *testBlockChain) InsertChain(blocks types.Blocks) (error, error) {
	for _, block := range blocks {
		bc.chainHeadFeed.Send(core.ChainHeadEvent{Block: block})
	}
	return nil, nil
}

type testFeeder struct {
	mu    sync.RWMutex
	stats stats.Stats

	block      *types.Block
	txs        types.Transactions
	receipts   types.Receipts
	callFrames []*mamoru.CallFrame
}

func (f *testFeeder) Stats() stats.Stats {
	return f.stats
}

func (f *testFeeder) FeedBlock(block *types.Block) mamoru_sniffer.Block {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.block = block
	f.stats.MarkBlocks()

	return mamoru_sniffer.Block{}
}

func (f *testFeeder) FeedTransactions(_ *big.Int, _ uint64, txs types.Transactions, _ types.Receipts) []mamoru_sniffer.Transaction {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.txs = append(f.txs, txs...)
	f.stats.MarkTxs(uint64(len(txs)))

	return []mamoru_sniffer.Transaction{}
}

func (f *testFeeder) FeedEvents(receipts types.Receipts) []mamoru_sniffer.Event {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.receipts = append(f.receipts, receipts...)
	f.stats.MarkEvents(uint64(len(receipts)))
	return []mamoru_sniffer.Event{}
}

func (f *testFeeder) FeedCallTraces(callFrames []*mamoru.CallFrame, _ uint64) []mamoru_sniffer.CallTrace {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.callFrames = append(f.callFrames, callFrames...)
	f.stats.MarkCallTraces(uint64(len(callFrames)))
	return []mamoru_sniffer.CallTrace{}
}

func (f *testFeeder) Txs() types.Transactions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.txs
}

func (f *testFeeder) Receipts() types.Receipts {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.receipts
}

func (f *testFeeder) CallFrames() []*mamoru.CallFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callFrames
}

func transaction(nonce uint64, gaslimit uint64, key *ecdsa.PrivateKey) *types.Transaction {
	return pricedTransaction(nonce, gaslimit, big.NewInt(765625000), key)
}

func pricedTransaction(nonce uint64, gaslimit uint64, gasprice *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	tx, _ := types.SignTx(types.NewTransaction(nonce, common.Address{}, big.NewInt(100), gaslimit, gasprice, nil), types.HomesteadSigner{}, key)
	return tx
}

type statusProgressMock struct {
}

func (s *statusProgressMock) Progress() ethereum.SyncProgress {
	return ethereum.SyncProgress{CurrentBlock: 2, HighestBlock: 2}
}

func TestMempoolSniffer(t *testing.T) {
	_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")

	defer func() {
		_ = os.Unsetenv("MAMORU_SNIFFER_ENABLE")
	}()
	actual := os.Getenv("MAMORU_SNIFFER_ENABLE")
	assert.Equal(t, "true", actual)

	// mock connect to sniffer
	mamoru.SnifferConnectFunc = func() (*mamoru_sniffer.Sniffer, error) { return nil, nil }

	var (
		key, _     = crypto.GenerateKey()
		address    = crypto.PubkeyToAddress(key.PublicKey)
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
		engine     = ethash.NewFaker()
	)

	statedb.SetBalance(address, new(uint256.Int).SetUint64(params.Ether))

	bChain := &testBlockChain{gasLimit: 100000, statedb: statedb, chainHeadFeed: new(event.Feed), chainEventFeed: new(event.Feed), chainSideEventFeed: new(event.Feed), engine: engine}
	db := rawdb.NewMemoryDatabase()
	chainConfig := params.TestChainConfig

	var gspec = core.Genesis{
		Config: chainConfig,
		Alloc:  types.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
	}
	genesis := gspec.MustCommit(db, triedb.NewDatabase(db, triedb.HashDefaults))

	legacyPool := legacypool.New(testTxPoolConfig, bChain)
	pool, err := txpool.New(testTxPoolConfig.PriceLimit, bChain, []txpool.SubPool{legacyPool})
	assert.NoError(t, err)
	defer func(pool *txpool.TxPool) {
		assert.NoError(t, pool.Close())
	}(pool)

	var txsPending types.Transactions
	var txsQueued types.Transactions
	for j := 0; j < 2; j++ {
		//create pending transactions
		txsPending = append(txsPending, transaction(uint64(j), 1000000, key))
	}
	for j := 0; j < 2; j++ {
		//create queued transactions (nonce > current nonce)
		txsQueued = append(txsQueued, transaction(uint64(j+10), 1000000, key))
	}
	n := 2
	blocks, _ := core.GenerateChain(chainConfig, genesis, engine, db, n, func(i int, gen *core.BlockGen) {
		gen.SetCoinbase(testBankAddress)
	})

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer func() {
		cancelCtx()
	}()

	sniffer := mamoru.NewSniffer()
	feeder := &testFeeder{stats: stats.NewStatsTxpool()}
	memSniffer := NewTxPoolBackendSniffer(ctx, pool, bChain, params.TestChainConfig, feeder, sniffer)

	memSniffer.sniffer.SetDownloader(&statusProgressMock{})

	newTxsEvent := make(chan core.NewTxsEvent, 10)
	sub := memSniffer.txPool.SubscribeTransactions(newTxsEvent, true)
	defer sub.Unsubscribe()

	newChainHeadEvent := make(chan core.ChainHeadEvent, 10)
	sub2 := memSniffer.SubscribeChainHeadEvent(newChainHeadEvent)
	defer sub2.Unsubscribe()

	err1, err2 := bChain.InsertChain(blocks)
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	time.Sleep(50 * time.Millisecond)

	pool.Add(append(txsPending, txsQueued...), true, false)
	time.Sleep(50 * time.Millisecond)

	if err := validateEvents(newTxsEvent, 2); err != nil {
		t.Errorf("newTxsEvent original event firing failed: %v", err)
	}

	if err := validateChainHeadEvents(newChainHeadEvent, n); err != nil {
		t.Errorf("newChainHeadEvent original event firing failed: %v", err)
	}

	pending, queued := pool.Stats()
	assert.Equal(t, txsPending.Len(), pending)
	assert.Equal(t, txsQueued.Len(), queued)

	assert.Equal(t, n, len(blocks))
	assert.Equal(t, txsPending.Len(), feeder.Txs().Len(), "pending transaction len must be equals feeder transaction len")
	assert.Equal(t, txsPending.Len(), feeder.Receipts().Len(), "receipts len must be equal")
	assert.Equal(t, txsPending.Len(), len(feeder.CallFrames()), "CallFrames len must be equal")

	callTraces := feeder.CallFrames()
	for _, call := range callTraces {
		assert.Empty(t, call.Error, "error must be empty")
		assert.NotNil(t, call.Type, "type must be not nil")
		assert.Equal(t, addrToHex(address), strings.ToLower(call.From), "address must be equal")
		assert.Equal(t, uint32(0), call.TxIndex, "tx index must be equal")
		assert.Equal(t, uint32(0), call.Depth, "depth must be equal")
	}

	assert.Equal(t, uint64(0), feeder.Stats().GetBlocks(), "blocks must be equal")
	assert.Equal(t, uint64(feeder.Txs().Len()), feeder.Stats().GetTxs(), "txs must be equal")
	assert.Equal(t, uint64(feeder.Receipts().Len()), feeder.Stats().GetEvents(), "events must be equal")
	assert.Equal(t, uint64(len(feeder.CallFrames())), feeder.Stats().GetTraces(), "call traces must be equal")
}

// validateEvents checks that the correct number of transaction addition events
// were fired on the pool's event feed.
func validateEvents(events chan core.NewTxsEvent, count int) error {
	var received []*types.Transaction

	for len(received) < count {
		select {
		case ev := <-events:
			received = append(received, ev.Txs...)
		case <-time.After(time.Second):
			return fmt.Errorf("event #%d not fired", len(received))
		}
	}
	if len(received) > count {
		return fmt.Errorf("more than %d events fired: %v", count, received[count:])
	}
	select {
	case ev := <-events:
		return fmt.Errorf("more than %d events fired: %v", count, ev.Txs)

	case <-time.After(50 * time.Millisecond):
		// This branch should be "default", but it's a data race between goroutines,
		// reading the event channel and pushing into it, so better wait a bit ensuring
		// really nothing gets injected.
	}
	return nil
}

func validateChainHeadEvents(events chan core.ChainHeadEvent, count int) error {
	var received []*types.Block

	for len(received) < count {
		select {
		case ev := <-events:
			received = append(received, ev.Block)
		case <-time.After(time.Second):
			return fmt.Errorf("event #%d not fired", len(received))
		}
	}
	if len(received) > count {
		return fmt.Errorf("more than %d events fired: %v", count, received[count:])
	}
	select {
	case ev := <-events:
		return fmt.Errorf("more than %d events fired: %v", count, ev.Block)

	case <-time.After(50 * time.Millisecond):
		// This branch should be "default", but it's a data race between goroutines,
		// reading the event channel and pushing into it, so better wait a bit ensuring
		// really nothing gets injected.
	}
	return nil
}

func addrToHex(a common.Address) string {
	return strings.ToLower(a.Hex())
}
