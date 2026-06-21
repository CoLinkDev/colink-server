package model

import (
	"time"

	"github.com/google/uuid"
)

type WsTicket struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;not null"`
	DeviceID  uuid.UUID `gorm:"type:uuid;not null"`
	Ticket    string    `gorm:"size:128;not null;uniqueIndex:idx_ws_tickets_ticket"`
	Consumed  bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null;index:idx_ws_tickets_expires_at"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}
