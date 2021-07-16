package engine

import (
	"fmt"
	stdlog "log"
	"os"
)

var (
	log *filteredLogger
)

type filteredLogger struct {
	logger *stdlog.Logger
}

func (f *filteredLogger) filteredArg(v ...interface{}) []interface{} {
	for idx, arg := range v {
		if s, ok := arg.(string); ok && len(s) == 40 {
			v[idx] = fmt.Sprintf("[%s..]", s[:6])
		}
		if s, ok := arg.(taskType); ok {
			if s == taskTorrent {
				v[idx] = "[Torrent]"
			} else {
				v[idx] = "[Magnet]"
			}
		}
	}

	return v
}

func (f *filteredLogger) Println(v ...interface{}) {
	f.logger.Println(f.filteredArg(v...)...)
}
func (f *filteredLogger) Printf(format string, v ...interface{}) {
	f.logger.Printf(format, f.filteredArg(v...)...)
}
func (f *filteredLogger) Fatal(v ...interface{}) {
	f.logger.Fatal(f.filteredArg(v...)...)
}
func (f *filteredLogger) Panic(v ...interface{}) {
	f.logger.Panicln(f.filteredArg(v...)...)
}

func init() {
	log = &filteredLogger{
		logger: stdlog.New(os.Stdout, "[engine]", stdlog.LstdFlags|stdlog.Lmsgprefix),
	}
}

func SetLoggerFlag(flag int) {
	log.logger.SetFlags(flag)
}
