package logger

import (
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

type Logger = *log.Logger

var global Logger

func Init(level string) {
	global = New(level)
}

// InitWithLevel sets the global logger level.
func InitWithLevel(level string) {
	Init(level)
}

func L() Logger {
	if global == nil {
		global = New("info")
	}
	return global
}

func New(level string) Logger {
	logger := log.NewWithOptions(os.Stdout, log.Options{
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02 15:04:05",
	})

	switch strings.ToLower(level) {
	case "debug":
		logger.SetLevel(log.DebugLevel)
	case "warn":
		logger.SetLevel(log.WarnLevel)
	case "error":
		logger.SetLevel(log.ErrorLevel)
	default:
		logger.SetLevel(log.InfoLevel)
	}

	return logger
}
