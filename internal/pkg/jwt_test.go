package pkg

import (
	"testing"
	"time"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	token, err := GenerateAccessToken("secret", "user-1", time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	claims, err := ParseAccessToken("secret", token)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}

	if claims.UserID != "user-1" {
		t.Fatalf("claims.UserID = %q, want %q", claims.UserID, "user-1")
	}
}
