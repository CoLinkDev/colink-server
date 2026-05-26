package service

import (
	"testing"
	"time"
)

func TestAllowTicketIssueRemovesExpiredHistory(t *testing.T) {
	service := &WsService{
		ticketLimitByID: map[string][]time.Time{
			"user-1": {time.Now().UTC().Add(-2 * time.Minute)},
		},
	}

	if !service.allowTicketIssue("user-1", time.Now().UTC()) {
		t.Fatal("expected ticket issue to be allowed")
	}

	if got := len(service.ticketLimitByID["user-1"]); got != 1 {
		t.Fatalf("expected one active timestamp after cleanup, got %d", got)
	}
}
