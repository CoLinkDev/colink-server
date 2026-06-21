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
	"colink-server/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
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

	if err := app.EnsureUpdateSchema(db); err != nil {
		log.Fatal("ensure update schema", zap.Error(err))
	}

	router, updateService := handler.NewUpdateRouter(cfg, db, log)
	bgCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	go worker.NewUpdateChecker(
		updateService,
		cfg.Update.CheckInterval,
		log,
	).Run(bgCtx)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("update server started", zap.Int("port", cfg.Server.Port), zap.String("mode", cfg.Server.Mode))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen update server", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopBackground()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown update server", zap.Error(err))
	}

	if err := app.CloseDatabase(sqlDB); err != nil {
		log.Error("close database", zap.Error(err))
	}
}
