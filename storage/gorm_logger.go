package storage

import (
  "fmt"

  "github.com/charmbracelet/log"
)

type gormLogAdapter struct {
  logger *log.Logger
}

func newGormLogger(logger *log.Logger) *gormLogAdapter {
  return &gormLogAdapter{logger: logger}
}

func (g *gormLogAdapter) Printf(format string, args ...any) {
  g.logger.Info(fmt.Sprintf(format, args...))
}
