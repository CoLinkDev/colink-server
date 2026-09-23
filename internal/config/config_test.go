package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDeviceLimit(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      int
		wantError bool
	}{
		{name: "default", value: "", want: 20},
		{name: "override", value: "25", want: 25},
		{name: "zero", value: "0", wantError: true},
		{name: "negative", value: "-1", wantError: true},
		{name: "not an integer", value: "many", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLINK_DEVICE_LIMIT", tt.value)

			cfg, err := Load()
			if tt.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.Device.Limit != tt.want {
				t.Fatalf("expected device limit %d, got %d", tt.want, cfg.Device.Limit)
			}
		})
	}
}

func TestLoadWebSocketDefaultsAndOverrides(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("COLINK_WS_TICKET_RATE_LIMIT", "")
		t.Setenv("COLINK_WS_MAX_MESSAGE_BYTES", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if cfg.WS.TicketRateLimit != 20 {
			t.Fatalf("expected ticket rate limit 20, got %d", cfg.WS.TicketRateLimit)
		}
		if cfg.WS.MaxMessageBytes != 8*1024*1024 {
			t.Fatalf("expected max message bytes %d, got %d", 8*1024*1024, cfg.WS.MaxMessageBytes)
		}
	})

	t.Run("overrides", func(t *testing.T) {
		t.Setenv("COLINK_WS_TICKET_RATE_LIMIT", "30")
		t.Setenv("COLINK_WS_MAX_MESSAGE_BYTES", "16777216")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if cfg.WS.TicketRateLimit != 30 {
			t.Fatalf("expected ticket rate limit 30, got %d", cfg.WS.TicketRateLimit)
		}
		if cfg.WS.MaxMessageBytes != 16777216 {
			t.Fatalf("expected max message bytes 16777216, got %d", cfg.WS.MaxMessageBytes)
		}
	})
}

func TestLoadRejectsInvalidTypedConfiguration(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "COLINK_SERVER_PORT", value: "not-a-port"},
		{key: "COLINK_SERVER_PORT", value: "65536"},
		{key: "COLINK_DATABASE_PORT", value: "0"},
		{key: "COLINK_DATABASE_CONNECT_TIMEOUT", value: "later"},
		{key: "COLINK_DATABASE_CONNECT_TIMEOUT", value: "0s"},
		{key: "COLINK_JWT_ACCESS_TTL", value: "forever"},
		{key: "COLINK_JWT_REFRESH_TTL", value: "-1h"},
		{key: "COLINK_WS_TICKET_TTL", value: "0s"},
		{key: "COLINK_WS_TICKET_RATE_LIMIT", value: "0"},
		{key: "COLINK_WS_TICKET_RATE_LIMIT", value: "many"},
		{key: "COLINK_WS_MAX_MESSAGE_BYTES", value: "-1"},
		{key: "COLINK_WS_MAX_MESSAGE_BYTES", value: "huge"},
		{key: "COLINK_UPDATE_CHECK_INTERVAL", value: "0s"},
		{key: "COLINK_NOTES_ATTACHMENT_RETENTION", value: "167h59m59s"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("expected error to mention %s, got %v", tt.key, err)
			}
		})
	}
}

func TestLoadRejectsAccessTTLLongerThanRefreshTTL(t *testing.T) {
	t.Setenv("COLINK_JWT_ACCESS_TTL", "73h")
	t.Setenv("COLINK_JWT_REFRESH_TTL", "72h")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "COLINK_JWT_ACCESS_TTL") {
		t.Fatalf("expected access TTL error, got %v", err)
	}
}

func TestLoadKeepsCurrentAccessTTLDefault(t *testing.T) {
	t.Setenv("COLINK_JWT_ACCESS_TTL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.JWT.AccessTTL != 72*time.Hour {
		t.Fatalf("expected access TTL %s, got %s", 72*time.Hour, cfg.JWT.AccessTTL)
	}
}
