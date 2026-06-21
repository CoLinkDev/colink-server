package app

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"colink-server/internal/config"
	"colink-server/internal/migration"
	"colink-server/internal/model"
)

func NewLogger(mode string) (*zap.Logger, error) {
	if mode == gin.ReleaseMode {
		return zap.NewProduction()
	}

	return zap.NewDevelopment()
}

func OpenDatabase(cfg *config.Config) (*gorm.DB, error) {
	gormLogger := logger.Default.LogMode(logger.Warn)
	if cfg.Server.Mode != gin.ReleaseMode {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	db, err := openDatabaseWithLogger(cfg, logger.Default.LogMode(logger.Silent))
	if err == nil {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		return openDatabaseWithLogger(cfg, gormLogger)
	}

	if ensureErr := ensureDatabaseExists(cfg); ensureErr != nil {
		return nil, fmt.Errorf("%w; ensure database: %v", err, ensureErr)
	}

	return openDatabaseWithLogger(cfg, gormLogger)
}

func CloseDatabase(db *sql.DB) error {
	return db.Close()
}

func RunMainMigrations(db *sql.DB, cfg *config.Config) error {
	return migration.Up(db, cfg.Database.DBName)
}

func EnsureUpdateSchema(db *gorm.DB) error {
	if err := ensurePgcrypto(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&model.AppRelease{},
		&model.ReleaseAsset{},
	); err != nil {
		return err
	}
	return execStatements(db,
		`DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_release_assets_release') THEN
				ALTER TABLE release_assets ADD CONSTRAINT fk_release_assets_release FOREIGN KEY (release_id) REFERENCES app_releases(id) ON DELETE CASCADE;
			END IF;
		END $$;`,
	)
}

func ensureDatabaseExists(cfg *config.Config) error {
	databaseName := strings.TrimSpace(cfg.Database.DBName)
	if databaseName == "" {
		return fmt.Errorf("database name is required")
	}
	if databaseName == "postgres" {
		return nil
	}

	adminDB, err := gorm.Open(postgres.Open(cfg.Database.DSNForDatabase("postgres")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open admin database: %w", err)
	}
	sqlDB, err := adminDB.DB()
	if err != nil {
		return fmt.Errorf("open admin sql database: %w", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	var exists bool
	if err := adminDB.Raw("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = ?)", databaseName).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := adminDB.Exec("CREATE DATABASE " + quoteIdentifier(databaseName)).Error; err != nil {
		return fmt.Errorf("create database %s: %w", databaseName, err)
	}
	return nil
}

func ensurePgcrypto(db *gorm.DB) error {
	return db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error
}

func execStatements(db *gorm.DB, statements ...string) error {
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func openDatabaseWithLogger(cfg *config.Config, log logger.Interface) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{
		Logger: log,
	})
}
