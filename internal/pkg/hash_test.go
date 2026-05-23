package pkg

import "testing"

func TestHashPasswordRoundTrip(t *testing.T) {
	password := "securepass123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if err := ComparePassword(hash, password); err != nil {
		t.Fatalf("ComparePassword() error = %v", err)
	}
}

func TestHashTokenStable(t *testing.T) {
	first := HashToken("token-value")
	second := HashToken("token-value")

	if first != second {
		t.Fatalf("HashToken() should be stable, got %q and %q", first, second)
	}
}
