package service

import (
	"strings"
	"testing"
)

func TestValidateDeviceName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "ascii", value: "Office PC"},
		{name: "unicode", value: "工作站"},
		{name: "one hundred unicode characters", value: strings.Repeat("设", 100)},
		{name: "one hundred and one unicode characters", value: strings.Repeat("设", 101), wantErr: true},
		{name: "empty", value: "   ", wantErr: true},
		{name: "invalid utf8", value: string([]byte{0xff}), wantErr: true},
		{name: "control character", value: "Office\nPC", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeviceName(tt.value)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected a valid device name, got %v", err)
			}
		})
	}
}
