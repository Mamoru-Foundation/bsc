package mamoru

import (
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

type CallStackFrame struct {
	Type     vm.OpCode        `json:"type"`
	From     common.Address   `json:"from"`
	Gas      uint64           `json:"gas"`
	GasUsed  uint64           `json:"gasUsed"`
	To       common.Address   `json:"to,omitempty" rlp:"optional"`
	Input    []byte           `json:"input" rlp:"optional"`
	Output   []byte           `json:"output,omitempty" rlp:"optional"`
	Error    string           `json:"error,omitempty" rlp:"optional"`
	Revertal string           `json:"revertReason,omitempty"`
	Calls    []CallStackFrame `json:"calls,omitempty" rlp:"optional"`
	Logs     []callLog        `json:"logs,omitempty" rlp:"optional"`
	// Placed at end on purpose. The RLP will be decoded to 0 instead of
	// nil if there are non-empty elements after in the struct.
	Value   *big.Int `json:"value,omitempty" rlp:"optional"`
	Depth   uint32
	TxIndex uint32
	TxHash  string
}

type callLog struct {
	Address common.Address `json:"address"`
	Topics  []common.Hash  `json:"topics"`
	Data    hexutil.Bytes  `json:"data"`
}

func (f *CallStackFrame) TypeString() string {
	return f.Type.String()
}

func (f *CallStackFrame) failed() bool {
	return len(f.Error) > 0
}

func (f *CallStackFrame) processOutput(output []byte, err error) {
	output = common.CopyBytes(output)
	if err == nil {
		f.Output = output
		return
	}
	f.Error = err.Error()
	if f.Type == vm.CREATE || f.Type == vm.CREATE2 {
		f.To = common.Address{}
	}
	if !errors.Is(err, vm.ErrExecutionReverted) || len(output) == 0 {
		return
	}
	f.Output = output
	if len(output) < 4 {
		return
	}
	if unpacked, err := abi.UnpackRevert(output); err == nil {
		f.Revertal = unpacked
	}
}

type CallStackTracer struct {
	TxsLen    int
	Source    string
	callstack []CallStackFrame
	Config    CallTracerConfig
	gasLimit  uint64
	interrupt uint32 // Atomic flag to signal execution interruption
	reason    error  // Textual reason for the interruption
	depth     atomic.Uint32
	txIndex   atomic.Uint32
	rndStr    string
	done      atomic.Int32

	txs types.Transactions
	env *vm.EVM

	sync.Mutex
}

var _ vm.EVMLogger = (*CallStackTracer)(nil)

// NewCallStackTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func NewCallStackTracer(txs types.Transactions, txLen int, Id string, OnlyTopCall bool, source string) *CallStackTracer {
	return &CallStackTracer{
		TxsLen:    txLen,
		callstack: []CallStackFrame{},
		Source:    source,
		Config: CallTracerConfig{
			OnlyTopCall: OnlyTopCall,
			WithLog:     false,
		},
		rndStr: Id,
		txs:    txs,
	}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *CallStackTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
	t.depth.Store(0)
	call := CallStackFrame{
		Type:    vm.CALL,
		From:    from,
		To:      to,
		Input:   common.CopyBytes(input),
		Gas:     gas,
		Value:   value,
		Depth:   t.depth.Load(),
		TxIndex: t.txIndex.Load(),
	}
	if create {
		call.Type = vm.CREATE
	}
	t.Lock()
	defer t.Unlock()
	t.callstack = append(t.callstack, call)

	log.Debug("CaptureStart", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack))
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *CallStackTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	t.Lock()
	defer t.Unlock()
	txIndex := t.txIndex.Load()
	// Skip if transaction not have gas or something wrong
	if txIndex >= uint32(len(t.callstack)) {
		return
	}

	t.callstack[txIndex].processOutput(output, err)
	t.depth.Store(0)

	log.Debug("CaptureEnd", "source", t.Source, "ID", t.rndStr, "txIndex", txIndex, "txLen", t.TxsLen, "callstack", len(t.callstack))
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *CallStackTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	// Only logs need to be captured via opcode processing
	if !t.Config.WithLog {
		return
	}
	// Avoid processing nested calls when only caring about top call
	if t.Config.OnlyTopCall && depth > 0 {
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
		if len(stackData) < 2 {
			return
		}
		// Don't modify the stack
		mStart := stackData[len(stackData)-1]
		mSize := stackData[len(stackData)-2]
		topics := make([]common.Hash, size)
		for i := 0; i < size; i++ {
			topic := stackData[len(stackData)-2-(i+1)]
			topics[i] = common.Hash(topic.Bytes32())
		}

		data := scope.Memory.GetCopy(int64(mStart.Uint64()), int64(mSize.Uint64()))
		logCall := callLog{Address: scope.Contract.Address(), Topics: topics, Data: hexutil.Bytes(data)}
		t.callstack[len(t.callstack)-1].Logs = append(t.callstack[len(t.callstack)-1].Logs, logCall)
	}
	log.Debug("CaptureState", "source", t.Source, "ID", t.rndStr, "txLen", t.TxsLen, "callstack", len(t.callstack))
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *CallStackTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.Config.OnlyTopCall {
		return
	}
	// Skip if tracing was interrupted
	if atomic.LoadUint32(&t.interrupt) > 0 {
		return
	}

	t.depth.Add(1)
	call := CallStackFrame{
		Type:  typ,
		From:  from,
		To:    to,
		Input: common.CopyBytes(input),
		Gas:   gas,
		Value: value,
		Depth: t.depth.Load(),
	}
	t.Lock()
	defer t.Unlock()
	t.callstack = append(t.callstack, call)
	log.Debug("CaptureEnter", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack))
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *CallStackTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if t.Config.OnlyTopCall {
		return
	}

	t.Lock()
	defer t.Unlock()

	t.depth.Add(^uint32(0))
	size := len(t.callstack)
	if size == 0 {
		return
	}

	// pop call
	call := t.callstack[size-1]
	t.callstack = t.callstack[:size-1]

	call.GasUsed = gasUsed
	call.processOutput(output, err)

	log.Debug("CaptureExit", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack))
	if len(t.callstack) > 0 { // Check if the slice is still not empty
		t.callstack[len(t.callstack)-1].Calls = append(t.callstack[len(t.callstack)-1].Calls, call)
	}
}

func (t *CallStackTracer) CaptureTxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
	var txHash string
	if len(t.txs) > 0 {
		txHash = t.txs[t.txIndex.Load()].Hash().String()
	}

	log.Debug("CaptureTxStart", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack), "txHash", txHash)
}

func (t *CallStackTracer) CaptureTxEnd(restGas uint64) {
	t.Lock()
	defer t.Unlock()

	txIndex := t.txIndex.Load()
	// Skip if transaction not have gas or something wrong
	if txIndex >= uint32(len(t.callstack)) {
		return
	}

	t.callstack[t.txIndex.Load()].GasUsed = t.gasLimit - restGas

	if t.Config.WithLog {
		// Logs are not emitted when the call fails
		clearFailedLogs(&t.callstack[t.txIndex.Load()], false)
	}

	currentTxIndex := t.txIndex.Load()
	t.txIndex.Add(1)
	var txHash string
	if len(t.txs) > 0 {
		txHash = t.txs[currentTxIndex].Hash().String()
		if len(t.callstack) > int(currentTxIndex) {
			t.callstack[currentTxIndex].TxHash = txHash
		}

		txHash = ""
		// Next tx hash
		if uint32(len(t.txs)-1) >= t.txIndex.Load() {
			txHash = t.txs[t.txIndex.Load()].Hash().String()
		}
	}

	log.Debug("CaptureTxEnd", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack), "txHash", txHash)
}

func (t *CallStackTracer) CaptureSystemTxEnd(intrinsicGas uint64) {
	if t.txIndex.Load() > uint32(len(t.callstack)) {
		return
	}

	t.callstack[t.txIndex.Load()].GasUsed -= intrinsicGas
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *CallStackTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, depth int, err error) {
	// Skip if tracing was interrupted
	if atomic.LoadUint32(&t.interrupt) > 0 {
		return
	}
	// Skip if not a call
	if len(t.callstack) == 0 {
		return
	}

	t.Lock()
	defer t.Unlock()
	t.callstack[len(t.callstack)-1].Error = err.Error()
	log.Debug("CaptureFault", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack), "err", err.Error())
}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *CallStackTracer) GetResult() (json.RawMessage, error) {
	callstack, err := t.TakeResult()
	if err != nil {
		return nil, err
	}

	res, err := MarshalJSON(callstack)
	if err != nil {
		return nil, err
	}

	log.Info("GetResult", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack))

	return json.RawMessage(res), t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *CallStackTracer) Stop(err error) {
	t.reason = err
	atomic.StoreUint32(&t.interrupt, 1)
}

// clearFailedLogs clears the logs of a callframe and all its children
// in case of execution failure.
func clearFailedLogs(cf *CallStackFrame, parentFailed bool) {
	failed := cf.failed() || parentFailed
	// Clear own logs
	if failed {
		cf.Logs = nil
	}
	for i := range cf.Calls {
		clearFailedLogs(&cf.Calls[i], failed)
	}
}

func (t *CallStackTracer) TakeResult() ([]*CallFrame, error) {
	defer t.cleanup() // Clean up and return the result.

	for i := range t.txs {
		var callTxHash string
		if len(t.callstack) > i {
			callTxHash = t.callstack[i].TxHash
			t.callstack[i].TxIndex = uint32(i)
			log.Info("TakeResult", "source", t.Source, "ID", t.rndStr, "Index", i, "txLen", t.TxsLen, "callstack", len(t.callstack), "txHash", t.txs[i].Hash().String())
		}
		if callTxHash != t.txs[i].Hash().String() {
			log.Debug("TakeResult System Tx", "source", t.Source, "ID", t.rndStr, "Index", i, "txLen", t.TxsLen, "callstack", len(t.callstack), "txHash", t.txs[i].Hash().String())
		}
	}

	frames := toFlatten(t.callstack)

	log.Info("TakeResult", "source", t.Source, "ID", t.rndStr, "txIndex", t.txIndex.Load(), "txLen", t.TxsLen, "callstack", len(t.callstack))

	return frames, t.reason
}

func (t *CallStackTracer) cleanup() {
	t.callstack = []CallStackFrame{}
	atomic.StoreUint32(&t.interrupt, 0)
	t.txIndex.Store(0)
	t.reason = nil
	t.done.Store(0)
}

func toFlatten(callstack []CallStackFrame) []*CallFrame {
	var frames []*CallFrame // TODO cast to another type
	for _, call := range callstack {
		rcall := call
		rcall.Calls = nil
		var value = uint64(0)
		if rcall.Value != nil {
			value = rcall.Value.Uint64()
		}
		frames = append(frames, &CallFrame{
			Type:         rcall.Type.String(),
			From:         addrToHex(rcall.From),
			To:           addrToHex(rcall.To),
			Value:        value,
			Gas:          rcall.Gas,
			GasUsed:      rcall.GasUsed,
			Input:        rcall.Input,
			Output:       string(rcall.Output),
			Error:        rcall.Error,
			RevertReason: rcall.Revertal,
			Depth:        rcall.Depth,
			TxIndex:      rcall.TxIndex,
			Logs:         rcall.Logs,
		})

		if len(call.Calls) > 0 {
			frames = append(frames, toFlatten(call.Calls)...)
		}
	}

	return frames
}

// MarshalJSON marshals as JSON.
func MarshalJSON(callstack []*CallFrame) ([]byte, error) {
	type callFrame0 struct {
		From         string         `json:"from"`
		Gas          hexutil.Uint64 `json:"gas"`
		GasUsed      hexutil.Uint64 `json:"gasUsed"`
		To           string         `json:"to,omitempty" rlp:"optional"`
		Input        hexutil.Bytes  `json:"input" rlp:"optional"`
		Output       hexutil.Bytes  `json:"output,omitempty" rlp:"optional"`
		Error        string         `json:"error,omitempty" rlp:"optional"`
		RevertReason string         `json:"revertReason,omitempty"`
		Calls        []CallFrame    `json:"calls,omitempty" rlp:"optional"`
		Logs         []callLog      `json:"logs,omitempty" rlp:"optional"`
		Value        uint64         `json:"value,omitempty" rlp:"optional"`
		Type         string         `json:"type"`
	}

	var encs []callFrame0
	for _, call := range callstack {
		var enc callFrame0
		enc.Type = call.Type
		enc.From = call.From
		enc.Gas = hexutil.Uint64(call.Gas)
		enc.GasUsed = hexutil.Uint64(call.GasUsed)
		enc.To = call.To
		enc.Input = call.Input
		enc.Output = hexutil.Bytes(call.Output)
		enc.Error = call.Error
		enc.RevertReason = call.RevertReason
		enc.Logs = call.Logs
		enc.Value = call.Value

		encs = append(encs, enc)
	}

	return json.Marshal(&encs)
}
