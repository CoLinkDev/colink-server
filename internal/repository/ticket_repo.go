package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"colink-server/internal/model"
)

type TicketRepository struct {
	db *gorm.DB
}

func NewTicketRepository(db *gorm.DB) *TicketRepository {
	return &TicketRepository{db: db}
}

func (r *TicketRepository) Create(ticket *model.WsTicket) error {
	return r.db.Create(ticket).Error
}

func (r *TicketRepository) ConsumeValid(ticketValue string, now time.Time) (*model.WsTicket, error) {
	var ticket model.WsTicket
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("ticket = ?", ticketValue).
			First(&ticket).
			Error; err != nil {
			return err
		}

		if ticket.Consumed || !ticket.ExpiresAt.After(now) {
			return gorm.ErrRecordNotFound
		}

		return tx.Model(&model.WsTicket{}).
			Where("id = ?", ticket.ID).
			Update("consumed", true).
			Error
	})
	if err != nil {
		return nil, err
	}

	ticket.Consumed = true
	return &ticket, nil
}

func (r *TicketRepository) Cleanup(now time.Time) error {
	return r.db.Where("consumed = ? OR expires_at <= ?", true, now).Delete(&model.WsTicket{}).Error
}
