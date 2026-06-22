package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"colink-server/internal/app"
	"colink-server/internal/config"
	"colink-server/internal/handler"
)

type testApp struct {
	cfg    *config.Config
	db     *gorm.DB
	sqlDB  *sql.DB
	server *httptest.Server
}

type envelope[T any] struct {
	Code    int    `json:"code"`
	Data    T      `json:"data"`
	Message string `json:"message"`
}

type authResult struct {
	UserID           string `json:"userId"`
	Token            string `json:"token"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresIn        int64  `json:"expiresIn"`
	RefreshExpiresIn int64  `json:"refreshExpiresIn"`
}

type refreshResult struct {
	Token            string `json:"token"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresIn        int64  `json:"expiresIn"`
	RefreshExpiresIn int64  `json:"refreshExpiresIn"`
}

type registerDeviceResult struct {
	DeviceID string `json:"deviceId"`
}

type deviceItem struct {
	DeviceID           string     `json:"deviceId"`
	Name               string     `json:"name"`
	PublicKey          string     `json:"publicKey"`
	PublicKeyUpdatedAt *time.Time `json:"publicKeyUpdatedAt"`
}

type deviceListResult struct {
	Devices []deviceItem `json:"devices"`
}

type ticketResult struct {
	Ticket string `json:"ticket"`
}

func TestAuthFlow(t *testing.T) {
	app := newTestApp(t, 5*time.Second)
	defer app.close()

	register := decodeOK[authResult](t, app.request(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email":    "alice@example.com",
		"username": "alice",
		"password": "password123",
	}))

	login := decodeOK[authResult](t, app.request(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": "alice",
		"password":   "password123",
	}))

	refreshed := decodeOK[refreshResult](t, app.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": login.RefreshToken,
	}))
	if refreshed.ExpiresIn <= 0 || refreshed.RefreshExpiresIn <= 0 {
		t.Fatalf("expected positive token TTLs, got access=%d refresh=%d", refreshed.ExpiresIn, refreshed.RefreshExpiresIn)
	}

	replayed := decodeOK[refreshResult](t, app.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": login.RefreshToken,
	}))
	if replayed.Token != refreshed.Token || replayed.RefreshToken != refreshed.RefreshToken {
		t.Fatal("expected refresh token replay to return the original refresh result")
	}

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/change-password", bearer(refreshed.Token), map[string]string{
		"oldPassword": "password123",
		"newPassword": "password456",
	}), http.StatusOK)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/logout", bearer(refreshed.Token), map[string]string{
		"refreshToken": refreshed.RefreshToken,
	}), http.StatusOK)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": "alice",
		"password":   "password123",
	}), http.StatusUnauthorized)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": "alice@example.com",
		"password":   "password456",
	}), http.StatusOK)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": register.RefreshToken,
	}), http.StatusOK)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": refreshed.RefreshToken,
	}), http.StatusUnauthorized)
}

func TestDeviceFlow(t *testing.T) {
	app := newTestApp(t, 5*time.Second)
	defer app.close()

	owner := decodeOK[authResult](t, app.request(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email":    "owner@example.com",
		"username": "owner",
		"password": "password123",
	}))
	other := decodeOK[authResult](t, app.request(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email":    "other@example.com",
		"username": "other",
		"password": "password123",
	}))

	device := decodeOK[registerDeviceResult](t, app.request(http.MethodPost, "/api/v1/devices", bearer(owner.Token), map[string]string{
		"deviceId":  "11111111-1111-4111-8111-111111111111",
		"name":      "Office PC",
		"type":      "windows",
		"publicKey": "QUJDRA==",
	}))

	devices := decodeOK[deviceListResult](t, app.request(http.MethodGet, "/api/v1/devices", bearer(owner.Token), nil))
	if len(devices.Devices) != 1 || devices.Devices[0].DeviceID != device.DeviceID {
		t.Fatal("device list did not return the created device")
	}
	if devices.Devices[0].PublicKeyUpdatedAt == nil {
		t.Fatal("device list did not return public key update time")
	}
	createdKeyUpdatedAt := *devices.Devices[0].PublicKeyUpdatedAt

	expectStatus(t, app.request(http.MethodPut, "/api/v1/devices/"+device.DeviceID, bearer(owner.Token), map[string]string{
		"name": "Home PC",
	}), http.StatusOK)

	time.Sleep(5 * time.Millisecond)
	expectStatus(t, app.request(http.MethodPut, "/api/v1/devices/"+device.DeviceID+"/key", bearer(owner.Token), map[string]string{
		"publicKey": "RUZHSA==",
	}), http.StatusOK)

	devices = decodeOK[deviceListResult](t, app.request(http.MethodGet, "/api/v1/devices", bearer(owner.Token), nil))
	if devices.Devices[0].PublicKey != "RUZHSA==" {
		t.Fatal("device list did not return rotated public key")
	}
	if devices.Devices[0].PublicKeyUpdatedAt == nil || !devices.Devices[0].PublicKeyUpdatedAt.After(createdKeyUpdatedAt) {
		t.Fatal("device list did not return updated public key update time")
	}

	expectStatus(t, app.request(http.MethodDelete, "/api/v1/devices/"+device.DeviceID, bearer(other.Token), nil), http.StatusNotFound)
	expectStatus(t, app.request(http.MethodDelete, "/api/v1/devices/"+device.DeviceID, bearer(owner.Token), nil), http.StatusOK)

	devices = decodeOK[deviceListResult](t, app.request(http.MethodGet, "/api/v1/devices", bearer(owner.Token), nil))
	if len(devices.Devices) != 0 {
		t.Fatal("expected device list to be empty after delete")
	}

	expectStatus(t, app.request(http.MethodPost, "/api/v1/devices", bearer(owner.Token), map[string]string{
		"deviceId":  "not-a-uuid",
		"name":      "Invalid",
		"type":      "windows",
		"publicKey": "QUJDRA==",
	}), http.StatusBadRequest)

	expectStatus(t, app.request(http.MethodPost, "/api/v1/devices", bearer(owner.Token), map[string]string{
		"deviceId":  "22222222-2222-4222-8222-222222222222",
		"name":      "First",
		"type":      "windows",
		"publicKey": "QUJDRA==",
	}), http.StatusOK)
	expectStatus(t, app.request(http.MethodPost, "/api/v1/devices", bearer(owner.Token), map[string]string{
		"deviceId":  "22222222-2222-4222-8222-222222222222",
		"name":      "Duplicate",
		"type":      "windows",
		"publicKey": "SUpLTA==",
	}), http.StatusOK)

	devices = decodeOK[deviceListResult](t, app.request(http.MethodGet, "/api/v1/devices", bearer(owner.Token), nil))
	if len(devices.Devices) != 1 || devices.Devices[0].Name != "Duplicate" || devices.Devices[0].PublicKey != "SUpLTA==" {
		t.Fatal("device upsert did not update existing device")
	}

	expectStatus(t, app.request(http.MethodPost, "/api/v1/devices", bearer(other.Token), map[string]string{
		"deviceId":  "22222222-2222-4222-8222-222222222222",
		"name":      "Stolen",
		"type":      "windows",
		"publicKey": "QUJDRA==",
	}), http.StatusConflict)
}

func TestWsTicketFlow(t *testing.T) {
	app := newTestApp(t, 50*time.Millisecond)
	defer app.close()

	auth := decodeOK[authResult](t, app.request(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email":    "ws@example.com",
		"username": "ws-user",
		"password": "password123",
	}))
	device := decodeOK[registerDeviceResult](t, app.request(http.MethodPost, "/api/v1/devices", bearer(auth.Token), map[string]string{
		"deviceId":  "33333333-3333-4333-8333-333333333333",
		"name":      "Laptop",
		"type":      "windows",
		"publicKey": "SUpLTA==",
	}))

	ticket := decodeOK[ticketResult](t, app.request(http.MethodPost, "/api/v1/ws/ticket", bearer(auth.Token), map[string]string{
		"deviceId": device.DeviceID,
	}))

	wsURL := "ws" + strings.TrimPrefix(app.server.URL, "http") + "/ws/v1?ticket=" + url.QueryEscape(ticket.Ticket)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	_ = resp.Body.Close()
	_ = conn.Close()

	expectStatus(t, app.request(http.MethodGet, "/ws/v1?ticket="+url.QueryEscape(ticket.Ticket), "", nil), http.StatusUnauthorized)

	expiring := decodeOK[ticketResult](t, app.request(http.MethodPost, "/api/v1/ws/ticket", bearer(auth.Token), map[string]string{
		"deviceId": device.DeviceID,
	}))
	time.Sleep(100 * time.Millisecond)
	expectStatus(t, app.request(http.MethodGet, "/ws/v1?ticket="+url.QueryEscape(expiring.Ticket), "", nil), http.StatusUnauthorized)
}

func newTestApp(t *testing.T, ticketTTL time.Duration) *testApp {
	t.Helper()

	dsn := os.Getenv("COLINK_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set COLINK_TEST_DATABASE_DSN to run integration tests")
	}

	dbName := parseDSNValue(dsn, "dbname")
	if dbName == "" {
		t.Fatal("COLINK_TEST_DATABASE_DSN must include dbname")
	}

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql database: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode},
		Database: config.DatabaseConfig{
			DBName: dbName,
		},
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		},
		WS: config.WSConfig{
			TicketTTL: ticketTTL,
		},
	}

	resetDatabase(t, sqlDB)
	if err := app.RunMainMigrations(sqlDB, cfg); err != nil {
		t.Fatalf("run main migrations: %v", err)
	}

	router := handler.NewMainRouter(cfg, db, zap.NewNop())
	server := httptest.NewServer(router)
	return &testApp{cfg: cfg, db: db, sqlDB: sqlDB, server: server}
}

func (a *testApp) close() {
	a.server.Close()
	_ = a.sqlDB.Close()
}

func (a *testApp) request(method string, path string, authorization string, body any) *http.Response {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, a.server.URL+path, reader)
	if err != nil {
		panic(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	return resp
}

func decodeOK[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var payload envelope[T]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload.Data
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", want, resp.StatusCode, string(body))
	}
}

func resetDatabase(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset database: %v", err)
	}
}

func parseDSNValue(dsn string, key string) string {
	for _, part := range strings.Fields(dsn) {
		name, value, ok := strings.Cut(part, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func bearer(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}
