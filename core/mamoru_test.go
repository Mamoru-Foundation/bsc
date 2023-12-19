package core

import (
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/mamoru"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/stretchr/testify/assert"

	statistics "github.com/ethereum/go-ethereum/mamoru/stats"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
)

// TestMamoruBlockchainTracer test mamoru blockchain tracer
func TestMamoruBlockchainTracer(t *testing.T) {
	t.Run("Blockchain Mamoru sniffer TRUE ", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")

		var (
			gendb   = rawdb.NewMemoryDatabase()
			key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
			address = crypto.PubkeyToAddress(key.PublicKey)
			funds   = big.NewInt(100000000000000000)
			gspec   = &Genesis{
				Config:  params.TestChainConfig,
				Alloc:   GenesisAlloc{address: {Balance: funds}},
				BaseFee: big.NewInt(params.InitialBaseFee),
			}
			genesis = gspec.MustCommit(gendb, trie.NewDatabase(gendb, trie.HashDefaults))
			signer  = types.LatestSigner(gspec.Config)
		)
		height := uint64(2)
		txlen := 10
		blocks, _ := GenerateChain(gspec.Config, genesis, ethash.NewFaker(), gendb, int(height), func(i int, block *BlockGen) {
			for j := 0; j < txlen; j++ {
				tx, err := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{0x00}, big.NewInt(int64(1000+j)), params.TxGas, block.header.BaseFee, nil), signer, key)
				if err != nil {
					panic(err)
				}
				block.AddTx(tx)
			}
		})
		// mock connect to sniffer
		mamoru.SnifferConnectFunc = func() (*mamoru_sniffer.Sniffer, error) { return nil, nil }

		var db = rawdb.NewMemoryDatabase()
		gspec.MustCommit(db, trie.NewDatabase(gendb, trie.HashDefaults))
		var blockchain, err = NewBlockChain(db, nil, gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Set the sniffer
		blockchain.Sniffer = mamoru.NewSniffer()
		feeder := NewTestFeeder(statistics.NewStatsBlockchain())
		// Set the feeder
		blockchain.MamoruFeeder = feeder

		defer blockchain.Stop()

		for _, block := range blocks {
			_, err = blockchain.InsertChain(types.Blocks{block})
			if err != nil {
				t.Log(err)
			}

			callTraces := feeder.CallFrames()
			for i, call := range callTraces {
				assert.Empty(t, call.Error, "error must be empty")
				assert.NotNil(t, call.Type, "type must be not nil")
				assert.Equal(t, addrToHexT(address), strings.ToLower(call.From), "address must be equal")
				assert.Equal(t, uint32(i), call.TxIndex, "tx index must be equal")
				assert.Equal(t, uint32(0), call.Depth, "depth must be equal")
			}

			assert.Equal(t, uint64(1), feeder.Stats().GetBlocks(), "blocks must be equal")
			assert.Equal(t, uint64(feeder.Txs().Len()), feeder.Stats().GetTxs(), "txs must be equal")
			assert.Equal(t, uint64(feeder.Receipts().Len()), feeder.Stats().GetEvents(), "events must be equal")
			assert.Equal(t, uint64(len(feeder.CallFrames())), feeder.Stats().GetTraces(), "call traces must be equal")
			// reset feeder
			feeder.Reset()
		}
	})
}

type TestFeeder struct {
	mu         sync.RWMutex
	stats      statistics.Stats
	block      []*types.Block
	txs        types.Transactions
	receipts   types.Receipts
	callFrames []*mamoru.CallFrame
}

func NewTestFeeder(stats statistics.Stats) *TestFeeder {
	return &TestFeeder{stats: stats}
}

func (f *TestFeeder) Stats() statistics.Stats {
	return f.stats
}

func (f *TestFeeder) FeedBlock(block *types.Block) mamoru_sniffer.Block {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.block = append(f.block, block)
	f.stats.MarkBlocks()

	return mamoru_sniffer.Block{}
}

func (f *TestFeeder) FeedTransactions(_ *big.Int, blockTime uint64, txs types.Transactions, _ types.Receipts) []mamoru_sniffer.Transaction {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.txs = append(f.txs, txs...)
	f.stats.MarkTxs(uint64(len(txs)))

	return []mamoru_sniffer.Transaction{}
}

func (f *TestFeeder) FeedEvents(receipts types.Receipts) []mamoru_sniffer.Event {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.receipts = append(f.receipts, receipts...)
	f.stats.MarkEvents(uint64(len(receipts)))

	return []mamoru_sniffer.Event{}
}

func (f *TestFeeder) FeedCallTraces(callFrames []*mamoru.CallFrame, _ uint64) []mamoru_sniffer.CallTrace {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.callFrames = append(f.callFrames, callFrames...)
	f.stats.MarkCallTraces(uint64(len(f.callFrames)))

	return []mamoru_sniffer.CallTrace{}
}

func (f *TestFeeder) Txs() types.Transactions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.txs
}

func (f *TestFeeder) Receipts() types.Receipts {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.receipts
}

func (f *TestFeeder) CallFrames() []*mamoru.CallFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callFrames
}

func (f *TestFeeder) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.block = nil
	f.txs = nil
	f.receipts = nil
	f.callFrames = nil
	f.stats.Reset()
}

func addrToHexT(a common.Address) string {
	return strings.ToLower(a.Hex())
}
