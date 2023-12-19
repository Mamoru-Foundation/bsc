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
	connectMutex       sync.Mutex
	sniffer            *mamoru_sniffer.Sniffer
	SnifferConnectFunc = mamoru_sniffer.Connect
)

const Delta = 10 // min diff between currentBlock and highestBlock

func mapToInterfaceSlice(m map[string]string) []interface{} {
	var result []interface{}
	for key, value := range m {
		result = append(result, key, value)
	}

	return result
}

func init() {
	mamoru_sniffer.InitLogger(func(entry mamoru_sniffer.LogEntry) {
		kvs := mapToInterfaceSlice(entry.Ctx)
		msg := "Mamoru core: " + entry.Message
		switch entry.Level {
		case mamoru_sniffer.LogLevelDebug:
			log.Debug(msg, kvs...)
		case mamoru_sniffer.LogLevelInfo:
			log.Info(msg, kvs...)
		case mamoru_sniffer.LogLevelWarning:
			log.Warn(msg, kvs...)
		case mamoru_sniffer.LogLevelError:
			log.Error(msg, kvs...)
		}
	})
}

type statusProgress interface {
	Progress() ethereum.SyncProgress
}

type Sniffer struct {
	mu     sync.Mutex
	status statusProgress
	synced bool
	delta  int64
}

func NewSniffer() *Sniffer {
	return &Sniffer{delta: getDeltaBlocks()}
}

func (s *Sniffer) SetDownloader(downloader statusProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = downloader
}

func (s *Sniffer) CheckRequirements() bool {
	return s.isSnifferEnable() && s.checkSynced() && connect()
}

func (s *Sniffer) checkSynced() bool {
	//if status is nil, we assume that node is synced
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
		if int64(progress.HighestBlock)-int64(progress.CurrentBlock) <= s.delta {
			s.synced = true
		}
		log.Info("Mamoru Sniffer sync", "syncing", s.synced, "current", int64(progress.CurrentBlock), "highest", int64(progress.HighestBlock))
		return s.synced
	}

	log.Info("Mamoru Sniffer sync", "syncing", s.synced, "CurrentBlock", int64(progress.CurrentBlock), "highest", int64(progress.HighestBlock))

	return false
}

func (s *Sniffer) isSnifferEnable() bool {
	val, ok := os.LookupEnv("MAMORU_SNIFFER_ENABLE")
	if !ok {
		return false
	}

	isEnable, err := strconv.ParseBool(val)
	if err != nil {
		log.Error("Mamoru Sniffer env parse error", "err", err)
		return false
	}

	return isEnable
}

func InitConnectFunc(f func() (*mamoru_sniffer.Sniffer, error)) {
	SnifferConnectFunc = f
}

func connect() bool {
	connectMutex.Lock()
	defer connectMutex.Unlock()

	if sniffer != nil {
		return true
	}

	var err error
	if sniffer == nil {
		sniffer, err = SnifferConnectFunc()
		if err != nil {
			erst := strings.Replace(err.Error(), "\t", "", -1)
			erst = strings.Replace(erst, "\n", "", -1)
			//	erst = strings.Replace(erst, " ", "", -1)
			log.Error("Mamoru Sniffer connect", "err", erst)

			return false
		}
	}

	return true
}

func getDeltaBlocks() int64 {
	delta := os.Getenv("MAMORU_SNIFFER_DELTA")
	if delta == "" {
		return Delta
	}

	deltaBlocks, err := strconv.ParseInt(delta, 10, 64)
	if err != nil {
		return Delta
	}

	return deltaBlocks
}
