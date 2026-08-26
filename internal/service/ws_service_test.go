package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"colink-server/internal/ws"
)

func TestAllowTicketIssueRemovesExpiredHistory(t *testing.T) {
	service := &WsService{
		ticketRateLimit: 20,
		ticketLimitByDeviceID: map[string][]time.Time{
			"device-1": {time.Now().UTC().Add(-2 * time.Minute)},
		},
	}

	if !service.allowTicketIssue("device-1", time.Now().UTC()) {
		t.Fatal("expected ticket issue to be allowed")
	}

	if got := len(service.ticketLimitByDeviceID["device-1"]); got != 1 {
		t.Fatalf("expected one active timestamp after cleanup, got %d", got)
	}
}

func TestAllowTicketIssueUsesConfiguredPerDeviceLimit(t *testing.T) {
	service := &WsService{
		ticketRateLimit: 2,
		ticketLimitByDeviceID: make(map[string][]time.Time),
	}
	now := time.Now().UTC()

	if !service.allowTicketIssue("device-1", now) {
		t.Fatal("expected first ticket for device-1 to be allowed")
	}
	if !service.allowTicketIssue("device-1", now) {
		t.Fatal("expected second ticket for device-1 to be allowed")
	}
	if service.allowTicketIssue("device-1", now) {
		t.Fatal("expected third ticket for device-1 to be rate limited")
	}
	if !service.allowTicketIssue("device-2", now) {
		t.Fatal("expected another device to have an independent limit")
	}
}

func TestHandleMessageBroadcastFansOutToSameUserExceptSender(t *testing.T) {
	hub := ws.NewHub()
	service := &WsService{hub: hub, lastSeenByID: make(map[uuid.UUID]time.Time)}
	userID := uuid.NewString()
	senderID := uuid.NewString()
	receiverOneID := uuid.NewString()
	receiverTwoID := uuid.NewString()

	sender, senderConn := newTestWsClient(t, hub, userID, senderID)
	_, receiverOneConn := newTestWsClient(t, hub, userID, receiverOneID)
	_, receiverTwoConn := newTestWsClient(t, hub, userID, receiverTwoID)
	_, otherUserConn := newTestWsClient(t, hub, uuid.NewString(), uuid.NewString())

	correlationID := "request-1"
	payload := json.RawMessage(`{"type":"clipboard.v1.sync","payload":{"contentType":"text/plain","content":"hello"}}`)
	service.HandleMessage(sender, ws.ClientMessage{
		ID:            "broadcast-1",
		Type:          "broadcast",
		CorrelationID: &correlationID,
		Payload:       payload,
	})

	assertBroadcastEnvelope(t, readEnvelope(t, receiverOneConn), "broadcast-1", senderID, correlationID, payload)
	assertBroadcastEnvelope(t, readEnvelope(t, receiverTwoConn), "broadcast-1", senderID, correlationID, payload)
	assertNoEnvelope(t, senderConn)
	assertNoEnvelope(t, otherUserConn)
}

func TestHandleMessageRelayPassesThroughCorrelationID(t *testing.T) {
	hub := ws.NewHub()
	service := &WsService{hub: hub, lastSeenByID: make(map[uuid.UUID]time.Time)}
	userID := uuid.NewString()
	senderID := uuid.NewString()
	receiverID := uuid.NewString()

	sender, _ := newTestWsClient(t, hub, userID, senderID)
	_, receiverConn := newTestWsClient(t, hub, userID, receiverID)

	correlationID := "request-2"
	payload := json.RawMessage(`{"type":"message.v1.text","payload":{"messageId":"message-1","text":"hello"}}`)
	service.HandleMessage(sender, ws.ClientMessage{
		ID:            "relay-1",
		Type:          "relay",
		To:            &receiverID,
		CorrelationID: &correlationID,
		Payload:       payload,
	})

	envelope := readEnvelope(t, receiverConn)
	if envelope.ID != "relay-1" {
		t.Fatalf("expected id %q, got %q", "relay-1", envelope.ID)
	}
	if envelope.Type != "relay" {
		t.Fatalf("expected relay type, got %q", envelope.Type)
	}
	if envelope.From == nil || *envelope.From != senderID {
		t.Fatalf("expected from %q, got %v", senderID, envelope.From)
	}
	if envelope.To == nil || *envelope.To != receiverID {
		t.Fatalf("expected to %q, got %v", receiverID, envelope.To)
	}
	if envelope.CorrelationID == nil || *envelope.CorrelationID != correlationID {
		t.Fatalf("expected correlationId %q, got %v", correlationID, envelope.CorrelationID)
	}
	assertEnvelopePayload(t, envelope.Payload, payload)
}

func TestHandleMessageBroadcastDropsEmptyPayload(t *testing.T) {
	hub := ws.NewHub()
	service := &WsService{hub: hub}
	userID := uuid.NewString()

	sender, _ := newTestWsClient(t, hub, userID, uuid.NewString())
	_, receiverConn := newTestWsClient(t, hub, userID, uuid.NewString())

	service.HandleMessage(sender, ws.ClientMessage{
		ID:   "broadcast-empty",
		Type: "broadcast",
	})

	assertNoEnvelope(t, receiverConn)
}

func TestHandleConnectedSendsBusinessVersionAndCatchup(t *testing.T) {
	hub := ws.NewHub()
	service := &WsService{hub: hub, lastSeenByID: make(map[uuid.UUID]time.Time)}
	userID := uuid.NewString()
	firstID := uuid.NewString()
	secondID := uuid.NewString()

	_, firstConn := newTestWsClient(t, hub, userID, firstID)
	second, secondConn := newTestWsClient(t, hub, userID, secondID)

	service.broadcastOnline(second)
	service.sendOnlineCatchup(second)

	broadcast := readEnvelope(t, firstConn)
	assertDeviceOnline(t, broadcast, secondID, "1.0.0")

	catchup := readEnvelope(t, secondConn)
	assertDeviceOnline(t, catchup, firstID, "1.0.0")
}

func TestValidateBusinessVersion(t *testing.T) {
	service := &WsService{}
	for _, version := range []string{"1.0.0", "not-semver"} {
		if err := service.ValidateBusinessVersion(version); err != nil {
			t.Fatalf("expected version %q to be valid: %v", version, err)
		}
	}
	for _, version := range []string{"", strings.Repeat("x", 65)} {
		if err := service.ValidateBusinessVersion(version); err == nil {
			t.Fatalf("expected version %q to be invalid", version)
		}
	}
}

func TestValidateCloudWebSocketVersion(t *testing.T) {
	service := &WsService{}
	for _, testCase := range []struct {
		version string
		want    string
	}{
		{"", "1.0.0"},
		{"not-semver", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"1.1.0", "1.1.0"},
	} {
		got, err := service.ValidateCloudWebSocketVersion(testCase.version)
		if err != nil || got != testCase.want {
			t.Fatalf("version %q: got %q, err=%v", testCase.version, got, err)
		}
	}

	if _, err := service.ValidateCloudWebSocketVersion("2.0.0"); err == nil {
		t.Fatal("expected incompatible major version to be rejected")
	}
}

func TestPushAcknowledgementMustUseSendingConnection(t *testing.T) {
	hub := ws.NewHub()
	service := &WsService{
		hub:           hub,
		pendingPushes: make(map[string]pendingPush),
	}
	userID := uuid.NewString()
	sender, _ := newTestWsClient(t, hub, userID, uuid.NewString())
	other, _ := newTestWsClient(t, hub, userID, uuid.NewString())
	pushID := "push-1"
	done := make(chan struct{})
	service.pendingPushes[pushID] = pendingPush{client: sender, done: done}

	service.HandleMessage(other, ws.ClientMessage{
		Type:          "notification.push-ack",
		CorrelationID: &pushID,
	})
	select {
	case <-done:
		t.Fatal("ack from another connection must be ignored")
	default:
	}

	service.HandleMessage(sender, ws.ClientMessage{
		Type:          "notification.push-ack",
		CorrelationID: &pushID,
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected matching ack to resolve pending push")
	}
}

func newTestWsClient(t *testing.T, hub *ws.Hub, userID string, deviceID string) (*ws.Client, *websocket.Conn) {
	t.Helper()

	clientCh := make(chan *ws.Client, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}

		client, err := ws.NewClient(conn, userID, deviceID, "test-device", "test", "1.0.0", "1.0.0", nil, 8*1024*1024, nil, nil)
		if err != nil {
			t.Errorf("new websocket client: %v", err)
			_ = conn.Close()
			return
		}
		clientCh <- client
	}))
	t.Cleanup(server.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	client := <-clientCh
	hub.Register(client)
	go client.WritePump()

	t.Cleanup(func() {
		client.Close()
		_ = conn.Close()
	})
	return client, conn
}

func assertDeviceOnline(t *testing.T, envelope ws.MessageEnvelope, from string, businessVersion string) {
	t.Helper()

	if envelope.Type != "device.online" {
		t.Fatalf("expected device.online type, got %q", envelope.Type)
	}
	if envelope.From == nil || *envelope.From != from {
		t.Fatalf("expected from %q, got %v", from, envelope.From)
	}
	payloadBytes, err := json.Marshal(envelope.Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var payload ws.DeviceOnlinePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.BusinessVersion != businessVersion {
		t.Fatalf("expected businessVersion %q, got %q", businessVersion, payload.BusinessVersion)
	}
}

func readEnvelope(t *testing.T, conn *websocket.Conn) ws.MessageEnvelope {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var envelope ws.MessageEnvelope
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("read websocket envelope: %v", err)
	}
	return envelope
}

func assertNoEnvelope(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var envelope ws.MessageEnvelope
	if err := conn.ReadJSON(&envelope); err == nil {
		t.Fatalf("expected no websocket envelope, got %+v", envelope)
	}
}

func assertBroadcastEnvelope(t *testing.T, envelope ws.MessageEnvelope, id string, from string, correlationID string, payload json.RawMessage) {
	t.Helper()

	if envelope.ID != id {
		t.Fatalf("expected id %q, got %q", id, envelope.ID)
	}
	if envelope.Type != "broadcast" {
		t.Fatalf("expected broadcast type, got %q", envelope.Type)
	}
	if envelope.From == nil || *envelope.From != from {
		t.Fatalf("expected from %q, got %v", from, envelope.From)
	}
	if envelope.To != nil {
		t.Fatalf("expected nil to, got %q", *envelope.To)
	}
	if envelope.CorrelationID == nil || *envelope.CorrelationID != correlationID {
		t.Fatalf("expected correlationId %q, got %v", correlationID, envelope.CorrelationID)
	}
	if envelope.Timestamp <= 0 {
		t.Fatalf("expected timestamp, got %d", envelope.Timestamp)
	}

	assertEnvelopePayload(t, envelope.Payload, payload)
}

func assertEnvelopePayload(t *testing.T, payloadValue any, expectedPayload json.RawMessage) {
	t.Helper()

	actualPayload, err := json.Marshal(payloadValue)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var actual any
	var expected any
	if err := json.Unmarshal(actualPayload, &actual); err != nil {
		t.Fatalf("unmarshal actual payload: %v", err)
	}
	if err := json.Unmarshal(expectedPayload, &expected); err != nil {
		t.Fatalf("unmarshal expected payload: %v", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected payload %s, got %s", expectedPayload, actualPayload)
	}
}
