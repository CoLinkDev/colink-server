package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
)

type stubUserRepository struct {
	user *model.User
	err  error
}

func (r stubUserRepository) FindByID(uuid.UUID) (*model.User, error) {
	return r.user, r.err
}

func TestRequireAuthChecksCurrentAccountStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	token, err := pkg.GenerateAccessToken("test-secret", userID.String(), time.Hour)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	tests := []struct {
		name       string
		repository stubUserRepository
		wantStatus int
	}{
		{
			name:       "active account",
			repository: stubUserRepository{user: &model.User{ID: userID}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "disabled account",
			repository: stubUserRepository{user: &model.User{ID: userID, Disabled: true}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing account",
			repository: stubUserRepository{err: errors.New("not found")},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/", NewAuthMiddleware("test-secret", tt.repository).RequireAuth(), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, response.Code)
			}
		})
	}
}
