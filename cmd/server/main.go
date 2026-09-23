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

	"colink-server/internal/app"
	"colink-server/internal/config"
	"colink-server/internal/handler"
	"colink-server/internal/janitor"
	"colink-server/internal/repository"
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

	log, err := app.NewLogger(cfg.Server.Mode)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = log.Sync()
	}()

	db, err := app.OpenDatabase(cfg)
	if err != nil {
		log.Fatal("open database", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("open sql database", zap.Error(err))
	}

	if err := app.RunMainMigrations(sqlDB, cfg); err != nil {
		log.Fatal("run main migrations", zap.Error(err))
	}

	router := handler.NewMainRouter(cfg, db, log)
	bgCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	go janitor.New(
		db,
		repository.NewTokenRepository(db),
		repository.NewTicketRepository(db),
		repository.NewNoteAttachmentRepository(db),
		repository.NewNoteChangeLogRepository(db),
		cfg.Notes,
		time.Hour,
		log,
	).Run(bgCtx)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		log.Info(
			"server started",
			zap.Int("port", cfg.Server.Port),
			zap.String("mode", cfg.Server.Mode),
			zap.Int("deviceLimit", cfg.Device.Limit),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen server", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopBackground()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown server", zap.Error(err))
	}

	if err := app.CloseDatabase(sqlDB); err != nil {
		log.Error("close database", zap.Error(err))
	}
}
