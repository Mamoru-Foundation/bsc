package mamoru

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/assert"

	tm "github.com/tendermint/tendermint/libs/common"
)

func TestCallStackTracer_CaptureEnter(t1 *testing.T) {
	t := NewCallStackTracer(nil, 2, tm.RandStr(3), false, "test")

	//Start Tx
	t.CaptureTxStart(0)
	// depth 0
	t.CaptureStart(nil, common.Address{}, common.Address{}, false, nil, 0, big.NewInt(0))
	// depth 1
	t.CaptureEnter(vm.CALL, common.Address{}, common.Address{}, []byte{}, 0, nil)
	// depth 2
	t.CaptureEnter(vm.CALL, common.Address{}, common.Address{}, []byte{}, 0, nil)
	t.CaptureExit([]byte{}, 0, nil)
	t.CaptureExit([]byte{}, 0, nil)

	t.CaptureEnd(nil, 0, nil)
	t.CaptureTxEnd(0)

	t.CaptureTxStart(0)
	t.CaptureStart(nil, common.Address{}, common.Address{}, false, nil, 0, big.NewInt(0))
	t.CaptureEnd(nil, 0, nil)
	t.CaptureTxEnd(0)

	result, err := t.TakeResult()
	assert.NoError(t1, err)
	assert.NotNil(t1, result)

	assert.Equal(t1, uint32(0), result[0].Depth)
	assert.Equal(t1, uint32(0), result[0].TxIndex)

	assert.Equal(t1, uint32(1), result[1].Depth)

	assert.Equal(t1, uint32(2), result[2].Depth)

	assert.Equal(t1, uint32(0), result[3].Depth)
	assert.Equal(t1, uint32(1), result[3].TxIndex)
}

func TestToFlatten(t1 *testing.T) {
	frames := []CallStackFrame{
		{Depth: 0,
			Calls: []CallStackFrame{
				{Depth: 1,
					Calls: []CallStackFrame{
						{Depth: 2, Calls: []CallStackFrame{}},
					},
				},
			},
		},
	}
	want := []CallStackFrame{
		{Depth: 0},
		{Depth: 1},
		{Depth: 2},
	}

	have := toFlatten(frames)
	for i := range have {
		assert.Equal(t1, want[i].Depth, have[i].Depth)
	}
}
