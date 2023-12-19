package mamoru

import (
	"fmt"
	"os"
	"testing"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/assert"
)

type StatusProgressMock struct {
	status ethereum.SyncProgress
}

func CreateProgress(currentBlock, highestBlock uint64) *StatusProgressMock {
	return &StatusProgressMock{
		status: ethereum.SyncProgress{CurrentBlock: currentBlock, HighestBlock: highestBlock},
	}
}

func (s *StatusProgressMock) Progress() ethereum.SyncProgress {
	return s.status
}

func TestSniffer_CheckRequirements(t *testing.T) {
	delta := 5
	tests := []struct {
		name   string
		status statusProgress
		want   bool
	}{
		{
			name:   "FALSE - currentBlock == 0 && highestBlock == 0",
			status: CreateProgress(0, 0),
			want:   false,
		},
		{
			name:   "FALSE - currentBlock < highestBlock",
			status: CreateProgress(1, 10),
			want:   false,
		},
		{
			name:   "TRUE - currentBlock == highestBlock",
			status: CreateProgress(10, 10),
			want:   true,
		},
		{
			name:   "TRUE - currentBlock > highestBlock",
			status: CreateProgress(20, 10),
			want:   true,
		},
		{
			name:   "TRUE - currentBlock == highestBlock - delta",
			status: CreateProgress(5, 10),
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Sniffer{
				status: tt.status,
				delta:  int64(delta),
			}
			if got := s.checkSynced(); got != tt.want {
				t.Errorf("CheckRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSniffer_isSnifferEnable(t *testing.T) {
	t.Run("TRUE env is set 1", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "1")
		defer unsetEnvSnifferEnable()
		s := NewSniffer()
		got := s.isSnifferEnable()
		assert.True(t, got)
	})
	t.Run("TRUE env is set true", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")
		defer unsetEnvSnifferEnable()
		s := NewSniffer()
		got := s.isSnifferEnable()
		assert.True(t, got)
	})
	t.Run("FALSE env is set 0", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "0")
		defer unsetEnvSnifferEnable()
		s := NewSniffer()
		got := s.isSnifferEnable()
		assert.False(t, got)
	})
	t.Run("FALSE env is set 0", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "false")
		defer unsetEnvSnifferEnable()
		s := NewSniffer()
		got := s.isSnifferEnable()
		assert.False(t, got)
	})
	t.Run("FALSE env is not set", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "")
		defer unsetEnvSnifferEnable()
		s := NewSniffer()
		got := s.isSnifferEnable()
		assert.False(t, got)
	})
}

func unsetEnvSnifferEnable() {
	_ = os.Unsetenv("MAMORU_SNIFFER_ENABLE")
}

func TestSniffer_connect(t *testing.T) {
	t.Run("TRUE ", func(t *testing.T) {
		InitConnectFunc(func() (*mamoru_sniffer.Sniffer, error) { return nil, nil })
		got := connect()
		assert.True(t, got)
	})
	t.Run("FALSE connect have error", func(t *testing.T) {
		InitConnectFunc(func() (*mamoru_sniffer.Sniffer, error) { return nil, fmt.Errorf("Some err") })
		got := connect()
		assert.False(t, got)
	})
}

func TestSniffer_CheckRequirements1(t *testing.T) {
	t.Run("TRUE ", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")
		defer unsetEnvSnifferEnable()
		InitConnectFunc(func() (*mamoru_sniffer.Sniffer, error) { return nil, nil })
		s := &Sniffer{
			status: CreateProgress(10, 5),
			synced: true,
		}
		assert.True(t, s.CheckRequirements())
	})
	t.Run("FALSE chain not sync ", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")
		defer unsetEnvSnifferEnable()
		SnifferConnectFunc = func() (*mamoru_sniffer.Sniffer, error) { return nil, nil }
		s := &Sniffer{
			status: CreateProgress(5, 10),
			synced: true,
		}
		assert.False(t, s.CheckRequirements())
	})
	t.Run("FALSE connect error", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "true")
		defer unsetEnvSnifferEnable()
		InitConnectFunc(func() (*mamoru_sniffer.Sniffer, error) { return nil, fmt.Errorf("Some err") })
		s := &Sniffer{
			status: CreateProgress(10, 5),
			synced: true,
		}
		assert.False(t, s.CheckRequirements())
	})
	t.Run("FALSE env not set", func(t *testing.T) {
		_ = os.Setenv("MAMORU_SNIFFER_ENABLE", "0")
		defer unsetEnvSnifferEnable()
		InitConnectFunc(func() (*mamoru_sniffer.Sniffer, error) { return nil, nil })
		s := &Sniffer{
			status: CreateProgress(10, 5),
			synced: true,
		}
		assert.False(t, s.CheckRequirements())
	})
}

func Test_getDeltaBlocks(t *testing.T) {
	t.Run("Success return delta from env ", func(t *testing.T) {
		want := int64(100)
		_ = os.Setenv("MAMORU_SNIFFER_DELTA", fmt.Sprintf("%d", want))
		defer os.Unsetenv("MAMORU_SNIFFER_DELTA")
		got := getDeltaBlocks()
		assert.Equal(t, want, got)
	})
	t.Run("Success return delta from env ", func(t *testing.T) {
		want := int64(Delta)
		got := getDeltaBlocks()
		assert.Equal(t, want, got)
		defer os.Unsetenv("MAMORU_SNIFFER_DELTA")
	})
}
