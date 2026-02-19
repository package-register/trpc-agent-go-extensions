package storage

import (
  "github.com/charmbracelet/log"
  "gorm.io/driver/sqlite"
  "gorm.io/gorm"
  gormLogger "gorm.io/gorm/logger"
)

type SQLiteConfig struct {
  Path   string
  Logger *log.Logger
}

func NewSQLite(cfg SQLiteConfig) (*gorm.DB, error) {
  loggerConfig := gormLogger.Config{
    SlowThreshold:             200000000,
    IgnoreRecordNotFoundError: true,
    LogLevel:                  gormLogger.Info,
  }

  gormLog := gormLogger.New(newGormLogger(cfg.Logger), loggerConfig)

  db, err := gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{
    Logger: gormLog,
  })
  if err != nil {
    return nil, err
  }

  return db, nil
}
