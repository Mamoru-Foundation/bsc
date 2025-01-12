package mamoru

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/log"
)

var (
	sniffer            *mamoru_sniffer.Sniffer
	SnifferConnectFunc = mamoru_sniffer.Connect
)

type statusProgress interface {
	Progress() ethereum.SyncProgress
}

type Sniffer struct {
	mu     sync.Mutex
	status statusProgress
	synced bool
}

func NewSniffer() *Sniffer {
	return &Sniffer{}
}

func (s *Sniffer) checkSynced() bool {
	// if status is nil, it means that the node is synced
	if s.status == nil {
		return true
	}

	progress := s.status.Progress()

	log.Info("Mamoru Sniffer sync", "syncing", s.synced, "diff", int64(progress.HighestBlock)-int64(progress.CurrentBlock))

	if progress.CurrentBlock < progress.HighestBlock {
		s.synced = false
	}
	if s.synced {
		return true
	}

	if progress.CurrentBlock > 0 && progress.HighestBlock > 0 {
		log.Info("Mamoru Sniffer sync", "current", progress.CurrentBlock)
		if int64(progress.HighestBlock)-int64(progress.CurrentBlock) <= 0 {
			s.synced = true
		}
		return s.synced
	}

	return false
}

func (s *Sniffer) SetDownloader(downloader statusProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = downloader
}

func (s *Sniffer) CheckRequirements() bool {
	return isSnifferEnable() && s.checkSynced() && connect()
}

func isSnifferEnable() bool {
	val, ok := os.LookupEnv("MAMORU_SNIFFER_ENABLE")
	if !ok {
		return false
	}
	isEnable, err := strconv.ParseBool(val)
	if err != nil {
		log.Error("Mamoru Sniffer env parse error", "err", err)
		return false
	}

	return ok && isEnable
}

func connect() bool {
	if sniffer != nil {
		return true
	}
	var err error
	if sniffer == nil {
		sniffer, err = SnifferConnectFunc()
		if err != nil {
			erst := strings.Replace(err.Error(), "\t", "", -1)
			erst = strings.Replace(erst, "\n", "", -1)
			log.Error("Mamoru Sniffer connect", "err", erst)
			return false
		}
	}
	return true
}
