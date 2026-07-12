// Package db initializes the GORM connection and runs auto-migration.
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open initializes the DB connection based on cfg.DBType and auto-migrates core models.
// SQLite uses the pure-Go glebarez driver (no CGO), keeping the build single-binary.
func Open(cfg *config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	var (
		db  *gorm.DB
		err error
	)
	switch cfg.DBType {
	case "postgres", "postgresql":
		if cfg.DSN == "" {
			return nil, fmt.Errorf("DATABASE_URL is required when DB_TYPE=postgres")
		}
		db, err = gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	default: // sqlite
		// WAL + busy_timeout + synchronous=NORMAL：并发写下避免 database is locked。
		// glebarez/sqlite 默认 rollback-journal + synchronous=FULL + busy_timeout=0，
		// 在 monitor(每10s写)/automation(cron写) 并发时会静默丢数据（service 层吞错）。
		dsn := cfg.SQLitePath() + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
		db, err = gorm.Open(sqlite.Open(dsn), gormCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.AutoMigrate(
		&models.User{}, &models.Site{}, &models.SiteStat{},
		&models.Downloader{}, &models.ActivationCode{}, &models.SystemConfig{},
		&models.PluginState{}, &models.PluginLog{}, &models.PluginKV{}, &models.PluginRelease{},
		&models.PosterCache{}, &models.CrossSeedSetting{}, &models.CrossSeedTask{},
		&models.FileStorage{}, &models.OpsCredential{}, &models.OpsConfig{}, &models.OpsConfigHistory{}, &models.RegistrationWatch{},
		&models.MonitorTarget{}, &models.MetricSnapshot{}, &models.MonitorAlert{},
		&models.OpsCommandRun{}, &models.OpsCommandRunTarget{},
		&models.SSHHostKey{}, &models.Agent{}, &models.AssessmentTask{}, &models.Medal{},
		&models.Notice{}, &models.Subscription{}, &models.SubscriptionHit{},
		&models.CheckinRecord{},
		&models.Workflow{}, &models.WorkflowRun{}, &models.RssSubscription{}, &models.RssItem{},
		&models.Webhook{},
		&models.BrushFlowTask{}, &models.BrushFlowSeed{},
		&models.ContractRule{},
		&models.MediaServer{},
	); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	return db, nil
}
