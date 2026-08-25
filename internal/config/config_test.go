package config

import "testing"

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
