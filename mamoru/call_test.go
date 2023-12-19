package mamoru

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/assert"
)

func TestCallTracer_CaptureEnter(t1 *testing.T) {
	t := NewCallTracer(false, CtxBlockchain)

	//Start Tx
	t.CaptureTxStart(0)
	// Depth 0 Top Call Start
	t.CaptureStart(nil, common.Address{}, common.Address{}, false, nil, 0, big.NewInt(0))

	// Depth 1
	t.CaptureEnter(vm.CALL, common.Address{}, common.Address{}, []byte{}, 0, nil)

	// Depth 2
	t.CaptureEnter(vm.CALL, common.Address{}, common.Address{}, []byte{}, 0, nil)
	t.CaptureExit([]byte{}, 0, nil)

	t.CaptureExit([]byte{}, 0, nil)

	// Depth 1
	t.CaptureEnter(vm.CALL, common.Address{}, common.Address{}, []byte{}, 0, nil)
	t.CaptureExit([]byte{}, 0, nil)

	//Top Call End
	t.CaptureEnd(nil, 0, nil)
	//End Tx
	t.CaptureTxEnd(0)

	//next tx start
	t.CaptureTxStart(0)

	t.CaptureStart(nil, common.Address{}, common.Address{}, false, nil, 0, big.NewInt(0))
	t.CaptureEnd(nil, 0, nil)

	t.CaptureTxEnd(0)

	result, err := t.TakeResult()
	assert.NoError(t1, err)

	assert.Equal(t1, uint32(0), result[0].Depth)
	assert.Equal(t1, uint32(0), result[0].TxIndex)
	assert.Equal(t1, uint32(1), result[1].Depth)
	assert.Equal(t1, uint32(2), result[2].Depth)
	assert.Equal(t1, uint32(1), result[3].Depth)
	assert.Equal(t1, uint32(0), result[3].TxIndex)

	assert.Equal(t1, uint32(0), result[4].Depth)
	assert.Equal(t1, uint32(1), result[4].TxIndex)
}
