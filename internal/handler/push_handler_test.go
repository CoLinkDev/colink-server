package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"colink-server/internal/middleware"
	"colink-server/internal/pkg"
	"colink-server/internal/service"
	"colink-server/internal/ws"
)

func TestPushHTTPStatusMatchesBarkBehavior(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{name: "unauthorized CoLink extension", code: pkg.CodeUnauthorized, want: http.StatusUnauthorized},
		{name: "invalid request body", code: pkg.CodeInvalidRequestBody, want: http.StatusBadRequest},
		{name: "invalid parameter", code: pkg.CodeInvalidParameter, want: http.StatusBadRequest},
		{name: "target device not found", code: pkg.CodeDeviceNotFound, want: http.StatusBadRequest},
		{name: "device offline", code: pkg.CodePushDeviceOffline, want: http.StatusInternalServerError},
		{name: "push not supported", code: pkg.CodePushNotSupported, want: http.StatusInternalServerError},
		{name: "push timeout", code: pkg.CodePushTimeout, want: http.StatusInternalServerError},
		{name: "internal error", code: pkg.CodeInternalError, want: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pushHTTPStatus(test.code); got != test.want {
				t.Fatalf("expected status %d, got %d", test.want, got)
			}
		})
	}
}

func TestParsePushRequestAcceptsJSONBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/push",
		strings.NewReader(`{"device_keys":["device-a","device-b"],"title":"Deploy","body":"completed","level":"active"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	request, targets, batch, err := parsePushRequest(context)
	if err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if !batch || len(targets) != 2 || targets[0] != "device-a" || targets[1] != "device-b" {
		t.Fatalf("unexpected targets: batch=%t targets=%v", batch, targets)
	}
	if request.Title == nil || *request.Title != "Deploy" || request.Body == nil || *request.Body != "completed" {
		t.Fatalf("unexpected payload: %+v", request.PushNotificationPayload)
	}
}

func TestParsePushRequestPathOverridesBodyTargetAndText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/push/device-in-path/path-title/path-body",
		strings.NewReader(`{"device_key":"device-in-body","title":"body-title","body":"body-text"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = []gin.Param{
		{Key: "path", Value: "/device-in-path/path-title/path-body"},
	}

	request, targets, batch, err := parsePushRequest(context)
	if err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if batch || len(targets) != 1 || targets[0] != "device-in-path" {
		t.Fatalf("unexpected path target: batch=%t targets=%v", batch, targets)
	}
	if request.Title == nil || *request.Title != "path-title" || request.Body == nil || *request.Body != "path-body" {
		t.Fatalf("path values did not take precedence: %+v", request.PushNotificationPayload)
	}
}

func TestPushRoutesRegisterWithoutPathConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	wsService := service.NewWsService(nil, nil, ws.NewHub(), time.Second, 20, nil)
	registerMainRoutes(
		router,
		NewAuthHandler(nil),
		NewDeviceHandler(nil),
		NewMeHandler(nil),
		NewWsHandler(wsService, 8*1024*1024),
		NewPushHandler(wsService),
		NewNoteHandler(nil),
		NewNoteTagHandler(nil),
		NewNoteAttachmentHandler(nil),
		NewSyncHandler(nil),
		middleware.NewAuthMiddleware("test-secret", nil),
	)
}
