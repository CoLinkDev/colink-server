package service

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"colink-server/internal/pkg"
)

func TestMapUserUniqueViolationUsesProtocolHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		wantCode   int
	}{
		{name: "email", constraint: "idx_users_email", wantCode: pkg.CodeEmailAlreadyExists},
		{name: "username", constraint: "idx_users_username", wantCode: pkg.CodeUsernameAlreadyExists},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appErr := mapUserUniqueViolation(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: test.constraint,
			})
			if appErr == nil {
				t.Fatal("expected mapped error")
			}
			if appErr.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, appErr.HTTPStatus)
			}
			if appErr.Code != test.wantCode {
				t.Fatalf("expected code %d, got %d", test.wantCode, appErr.Code)
			}
		})
	}
}
