package mamoru

import (
	"encoding/json"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cometbft/cometbft/libs/rand"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

type CallFrame struct {
	Type         string
	From         string
	To           string
	Value        uint64
	Gas          uint64
	GasUsed      uint64
	Input        []byte
	Output       string
	Error        string
	RevertReason string
	Depth        uint32
	TxIndex      uint32
	Logs         []callLog `json:"logs,omitempty" rlp:"optional"`
}

func (f CallFrame) failed() bool {
	return len(f.Error) > 0
}

func bytesToHex(s []byte) string {
	return "0x" + common.Bytes2Hex(s)
}

func addrToHex(a common.Address) string {
	return strings.ToLower(a.Hex())
}

type CallTracer struct {
	Source             string
	rndStr             string
	env                *vm.EVM
	CallList           []CallFrame
	Config             CallTracerConfig
	interrupt          uint32 // Atomic flag to signal execution interruption
	reason             error  // Textual reason for the interruption
	depth              atomic.Uint32
	txIndex            atomic.Uint32
	topCallIndex       atomic.Uint32
	gasLimit           uint64
	isStartFromTxStart bool

	mu sync.Mutex
}

func (t *CallTracer) CaptureSystemTxEnd(intrinsicGas uint64) {
	if t.topCallIndex.Load() > uint32(len(t.CallList)) {
		return
	}

	t.CallList[t.topCallIndex.Load()].GasUsed -= intrinsicGas
}

// NewCallTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func NewCallTracer(OnlyTopCall bool, source string) *CallTracer {
	// First callframe contains tx context info
	// and is populated on start and end.
	return &CallTracer{
		Source:   source,
		rndStr:   rand.Str(6),
		CallList: []CallFrame{},
		Config:   CallTracerConfig{OnlyTopCall: OnlyTopCall},
	}
}

func (t *CallTracer) GetResult() (json.RawMessage, error) {
	calls, err := t.TakeResult()
	if err != nil {
		return nil, err
	}

	res, err := json.Marshal(calls)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(res), t.reason
}

var _ vm.EVMLogger = (*CallTracer)(nil)

type CallTracerConfig struct {
	OnlyTopCall bool `json:"onlyTopCall"` // If true, call tracer won't collect any subcalls
	WithLog     bool `json:"withLog"`     // If true, call tracer will collect event logs
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *CallTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
	t.depth.Store(0)
	t.isStartFromTxStart = true

	call := CallFrame{
		Type:    "CALL",
		From:    addrToHex(from),
		To:      addrToHex(to),
		Input:   common.CopyBytes(input),
		Gas:     gas,
		Value:   value.Uint64(),
		Depth:   t.depth.Load(),
		TxIndex: t.txIndex.Load(),
	}
	if create {
		call.Type = "CREATE"
	}
	var blockNumber string
	if t.env != nil {
		blockNumber = t.env.Context.BlockNumber.String()
	}
	t.topCallIndex.Store(uint32(len(t.CallList)))
	log.Warn("CaptureStart", "randStr", t.rndStr, "source", t.Source, "blockNumber", blockNumber, "topIndex", t.topCallIndex.Load(), "startFromTxStart", t.isStartFromTxStart)
	t.CallList = append(t.CallList, call)
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *CallTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if len(t.CallList) < 1 || !t.isStartFromTxStart {
		return
	}

	t.depth.Store(0)
	var blockNumber string
	if t.env != nil {
		blockNumber = t.env.Context.BlockNumber.String()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	topCallIndex := t.topCallIndex.Load()
	log.Warn("CaptureEnd", "randStr", t.rndStr, "source", t.Source, "blockNumber", blockNumber, "len(CallList)", len(t.CallList), "topIndex", topCallIndex, "startFromTxStart", t.isStartFromTxStart)

	t.CallList[topCallIndex].GasUsed = gasUsed
	if err != nil {
		t.CallList[topCallIndex].Error = err.Error()
		if err.Error() == "execution reverted" && len(output) > 0 {
			t.CallList[topCallIndex].Output = bytesToHex(output)
		}
	} else {
		t.CallList[topCallIndex].Output = bytesToHex(output)
	}
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *CallTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	// Only logs need to be captured via opcode processing
	if !t.Config.WithLog {
		return
	}
	// Avoid processing nested calls when only caring about top call
	if t.Config.OnlyTopCall && t.depth.Load() > 0 {
		return
	}
	// Skip if tracing was interrupted
	if atomic.LoadUint32(&t.interrupt) > 0 {
		return
	}
	switch op {
	case vm.LOG0, vm.LOG1, vm.LOG2, vm.LOG3, vm.LOG4:
		size := int(op - vm.LOG0)

		stack := scope.Stack
		stackData := stack.Data()

		// Don't modify the stack
		mStart := stackData[len(stackData)-1]
		mSize := stackData[len(stackData)-2]
		topics := make([]common.Hash, size)
		for i := 0; i < size; i++ {
			topic := stackData[len(stackData)-2-(i+1)]
			topics[i] = common.Hash(topic.Bytes32())
		}

		data := scope.Memory.GetCopy(int64(mStart.Uint64()), int64(mSize.Uint64()))
		logs := callLog{Address: scope.Contract.Address(), Topics: topics, Data: hexutil.Bytes(data)}
		t.CallList[len(t.CallList)-1].Logs = append(t.CallList[len(t.CallList)-1].Logs, logs)
	}
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *CallTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, depth int, err error) {
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *CallTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.Config.OnlyTopCall || !t.isStartFromTxStart {
		return
	}

	// Skip if tracing was interrupted
	if atomic.LoadUint32(&t.interrupt) > 0 {
		t.env.Cancel()
		return
	}
	log.Warn("CaptureEnter", "randStr", t.rndStr, "source", t.Source, "len(CallList)", len(t.CallList), "topIndex", t.topCallIndex.Load(), "startFromTxStart", t.isStartFromTxStart)
	t.depth.Add(1)
	var valueC uint64
	if value != nil {
		valueC = value.Uint64()
	}

	call := CallFrame{
		Type:    typ.String(),
		From:    addrToHex(from),
		To:      addrToHex(to),
		Input:   common.CopyBytes(input),
		Gas:     gas,
		Depth:   t.depth.Load(),
		Value:   valueC,
		TxIndex: t.txIndex.Load(),
	}
	t.CallList = append(t.CallList, call)
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *CallTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if len(t.CallList) < 1 || !t.isStartFromTxStart {
		return
	}
	log.Warn("CaptureExit", "randStr", t.rndStr, "source", t.Source, "len(CallList)", len(t.CallList), "topIndex", t.topCallIndex.Load(), "startFromTxStart", t.isStartFromTxStart, "err", err)

	t.mu.Lock()
	defer t.mu.Unlock()

	if err != nil { //execution reverted
		t.CallList[len(t.CallList)-1].Error = err.Error()
		if err.Error() == "execution reverted" && len(output) > 0 {
			t.CallList[len(t.CallList)-1].Output = bytesToHex(output)
		}
	} else {
		// TODO; fix panic: runtime error: index out of range [130] with length 130
		t.CallList[len(t.CallList)-1].Output = bytesToHex(output)
		t.CallList[len(t.CallList)-1].GasUsed = gasUsed
	}

	t.depth.Add(^uint32(0))
}

func (t *CallTracer) CaptureTxStart(gasLimit uint64) {
	t.isStartFromTxStart = true
	t.gasLimit = gasLimit
	log.Warn("CaptureTxStart", "randStr", t.rndStr, "source", t.Source, "len(CallList)", len(t.CallList), "topIndex", t.topCallIndex.Load(), "startFromTxStart", t.isStartFromTxStart)
}

func (t *CallTracer) CaptureTxEnd(restGas uint64) {
	if len(t.CallList) < 1 || !t.isStartFromTxStart {
		return
	}
	txIndex := t.txIndex.Load()
	// Skip if transaction not have gas or something wrong
	if txIndex >= uint32(len(t.CallList)) {
		return
	}
	t.isStartFromTxStart = false
	t.txIndex.Add(1)
	log.Warn("CaptureTxEnd", "randStr", t.rndStr, "source", t.Source, "len(t.CallList)", len(t.CallList), "topIndex", t.topCallIndex.Load(), "startFromTxStart", t.isStartFromTxStart)
	t.mu.Lock()
	t.CallList[t.topCallIndex.Load()].GasUsed = t.gasLimit - restGas
	defer t.mu.Unlock()
	if t.Config.WithLog {
		for i := range t.CallList {
			failed := t.CallList[i].failed()
			// Clear own logs
			if failed {
				t.CallList[i].Logs = nil
			}
		}
	}
}

// TakeResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *CallTracer) TakeResult() ([]*CallFrame, error) {
	var frames []*CallFrame
	for _, call := range t.CallList {
		rcall := call
		frames = append(frames, &rcall)
	}

	defer func() {
		t.CallList = []CallFrame{}
		atomic.StoreUint32(&t.interrupt, 0)
		t.txIndex.Store(0)
		t.reason = nil
		t.isStartFromTxStart = false
		var blockNumber string
		if t.env != nil {
			blockNumber = t.env.Context.BlockNumber.String()
		}
		log.Warn("TakeResult", "randStr", t.rndStr, "source", t.Source, "blockNumber", blockNumber, "len(CallList)", len(t.CallList), "topIndex", t.topCallIndex.Load())
	}()

	return frames, t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *CallTracer) Stop(err error) {
	t.reason = err
	atomic.StoreUint32(&t.interrupt, 1)
}
