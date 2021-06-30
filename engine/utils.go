package engine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"strings"
	"sync"

	"github.com/c2h5oh/datasize"
	"golang.org/x/time/rate"
)

func rateLimiter(rstr string) (*rate.Limiter, error) {
	var rateSize int
	rstr = strings.ToLower(strings.TrimSpace(rstr))
	switch rstr {
	case "low":
		// ~50k/s
		rateSize = 50000
	case "medium":
		// ~500k/s
		rateSize = 500000
	case "high":
		// ~1500k/s
		rateSize = 1500000
	case "unlimited", "0", "":
		// unlimited
		return rate.NewLimiter(rate.Inf, 0), nil
	default:
		var v datasize.ByteSize
		err := v.UnmarshalText([]byte(rstr))
		if err != nil {
			return nil, err
		}
		if v > 2147483647 {
			// max of int, unlimited
			return nil, errors.New("excceed int val")
		}

		rateSize = int(v)
	}
	return rate.NewLimiter(rate.Limit(rateSize), rateSize*3), nil
}

var (
	log *filteredLogger
)

type filteredLogger struct {
	logger *stdlog.Logger
}

func (f *filteredLogger) Println(v ...interface{}) {
	for idx, arg := range v {
		if s, ok := arg.(string); ok && len(s) == 40 {
			v[idx] = fmt.Sprintf("[%s...]", s[:6])
		}
		if s, ok := arg.(taskType); ok {
			if s == taskTorrent {
				v[idx] = "[Torrent]"
			} else {
				v[idx] = "[Magnet]"
			}
		}
	}
	f.logger.Println(v...)
}
func (f *filteredLogger) Printf(format string, v ...interface{}) {
	for idx, arg := range v {
		if s, ok := arg.(string); ok && len(s) == 40 {
			v[idx] = fmt.Sprintf("%s...", s[:6])
		}
		if s, ok := arg.(taskType); ok {
			if s == taskTorrent {
				v[idx] = "[Torrent]"
			} else {
				v[idx] = "[Magnet]"
			}
		}
	}
	f.logger.Printf(format, v...)
}
func (f *filteredLogger) Fatal(v ...interface{}) {
	f.logger.Fatal(v...)
}

func init() {
	log = &filteredLogger{
		logger: stdlog.New(os.Stdout, "[engine]", stdlog.LstdFlags|stdlog.Lmsgprefix),
	}
}

func SetLoggerFlag(flag int) {
	log.logger.SetFlags(flag)
}

func cmdScanLine(p io.ReadCloser, wg *sync.WaitGroup, logprefix string) {
	sc := bufio.NewScanner(p)
	for sc.Scan() {
		oline := strings.TrimSpace(sc.Text())
		if len(oline) > 0 {
			log.Println(logprefix, oline)
		}
	}

	wg.Done()
}
