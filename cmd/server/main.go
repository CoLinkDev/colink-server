package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"colink-server/internal/config"
	"colink-server/internal/handler"
	"colink-server/internal/model"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	if cfg.JWT.Secret == "" {
		panic("jwt.secret is required")
	}

	gin.SetMode(cfg.Server.Mode)

	log, err := newLogger(cfg.Server.Mode)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = log.Sync()
	}()

	db, err := openDatabase(cfg)
	if err != nil {
		log.Fatal("open database", zap.Error(err))
	}

	if cfg.Server.Mode != gin.ReleaseMode {
		if err := autoMigrate(db); err != nil {
			log.Fatal("auto migrate", zap.Error(err))
		}
	}

	router := handler.NewRouter(cfg, db, log)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("server started", zap.Int("port", cfg.Server.Port), zap.String("mode", cfg.Server.Mode))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen server", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown server", zap.Error(err))
	}
}

func newLogger(mode string) (*zap.Logger, error) {
	if mode == gin.ReleaseMode {
		return zap.NewProduction()
	}

	return zap.NewDevelopment()
}

func openDatabase(cfg *config.Config) (*gorm.DB, error) {
	gormLogger := logger.Default.LogMode(logger.Warn)
	if cfg.Server.Mode != gin.ReleaseMode {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	return gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{
		Logger: gormLogger,
	})
}

func autoMigrate(db *gorm.DB) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		return err
	}

	return db.AutoMigrate(
		&model.User{},
		&model.Device{},
		&model.RefreshToken{},
		&model.WsTicket{},
	)
}
