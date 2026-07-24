// Package db initializes the GORM connection and runs auto-migration.
package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const subscriptionNullableBooleansMigrationKey = "MigrationSubscriptionNullableBooleansV1"

// Open initializes the DB connection based on cfg.DBType and auto-migrates core models.
// SQLite uses the pure-Go glebarez driver (no CGO), keeping the build single-binary.
func Open(cfg *config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		// IgnoreRecordNotFoundError：本仓库大量使用「先 First 再判 ErrRecordNotFound 决定插/更」
		// （如 SystemConfig.Set 的 upsert、Get 的读默认空值）模式，这类 NotFound 都是良性的。
		// GORM 默认 logger 会把它当告警以红色打印（吓人噪音，如 systemconfig.go:37 record not found），
		// 全局静默；真实 DB 错误（连接/约束/语法）仍照常打印。
		Logger: logger.New(
			log.New(os.Stdout, "", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			},
		),
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
		&models.User{}, &models.Site{}, &models.SiteStat{}, &models.SiteConnectivity{},
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
		&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{},
		&models.TransferHistory{}, &models.DownloadMediaLink{},
		&models.AgentConversation{}, &models.AgentMessage{},
	); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}

	if err := migrateSubscriptionNullableBooleans(db); err != nil {
		return nil, err
	}
	return db, nil
}

// migrateSubscriptionNullableBooleans converts historical false values to the new
// "inherit global default" state exactly once. New explicit false values must survive
// subsequent application restarts.
func migrateSubscriptionNullableBooleans(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.SystemConfig{}).
			Where("key = ?", subscriptionNullableBooleansMigrationKey).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check subscription nullable migration: %w", err)
		}
		if count > 0 {
			return nil
		}

		for _, col := range []string{"enable_rating_guard", "best_version"} {
			if err := tx.Model(&models.Subscription{}).
				Where(col+" = ?", false).
				Update(col, nil).Error; err != nil {
				return fmt.Errorf("migrate subscriptions.%s nullable: %w", col, err)
			}
		}
		if err := tx.Create(&models.SystemConfig{
			Key: subscriptionNullableBooleansMigrationKey, Value: "done",
		}).Error; err != nil {
			return fmt.Errorf("record subscription nullable migration: %w", err)
		}
		return nil
	})
}
