package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"colink-server/internal/pkg"
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

func TestParseDeviceUUID(t *testing.T) {
	if _, err := parseDeviceUUID("11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("expected a valid UUID v4, got %v", err)
	}

	for _, value := range []string{
		"not-a-uuid",
		"11111111-1111-1111-8111-111111111111",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := parseDeviceUUID(value)
			var appErr *pkg.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError, got %v", err)
			}
			if appErr.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, appErr.HTTPStatus)
			}
			if appErr.Code != pkg.CodeInvalidDeviceID {
				t.Fatalf("expected code %d, got %d", pkg.CodeInvalidDeviceID, appErr.Code)
			}
			if appErr.Message != "invalid device id" {
				t.Fatalf("expected invalid device id message, got %q", appErr.Message)
			}
		})
	}
}
