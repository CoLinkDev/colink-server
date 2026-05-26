package janitor

import (
	"context"
	"time"

	"go.uber.org/zap"

	"colink-server/internal/repository"
)

type Janitor struct {
	tokenRepo  *repository.TokenRepository
	ticketRepo *repository.TicketRepository
	interval   time.Duration
	log        *zap.Logger
}

func New(
	tokenRepo *repository.TokenRepository,
	ticketRepo *repository.TicketRepository,
	interval time.Duration,
	log *zap.Logger,
) *Janitor {
	return &Janitor{
		tokenRepo:  tokenRepo,
		ticketRepo: ticketRepo,
		interval:   interval,
		log:        log,
	}
}

func (j *Janitor) Run(ctx context.Context) {
	j.cleanup()

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.cleanup()
		}
	}
}

func (j *Janitor) cleanup() {
	now := time.Now().UTC()

	if err := j.tokenRepo.DeleteExpired(now); err != nil {
		j.log.Warn("cleanup refresh tokens", zap.Error(err))
	}
	if err := j.ticketRepo.Cleanup(now); err != nil {
		j.log.Warn("cleanup ws tickets", zap.Error(err))
	}
}
