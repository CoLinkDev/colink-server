package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"colink-server/internal/service"
)

type UpdateChecker struct {
	service  *service.UpdateService
	interval time.Duration
	log      *zap.Logger
}

func NewUpdateChecker(service *service.UpdateService, interval time.Duration, log *zap.Logger) *UpdateChecker {
	return &UpdateChecker{
		service:  service,
		interval: interval,
		log:      log,
	}
}

func (w *UpdateChecker) Run(ctx context.Context) {
	w.service.CheckForUpdates(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("update checker stopped")
			return
		case <-ticker.C:
			w.service.CheckForUpdates(ctx)
		}
	}
}
