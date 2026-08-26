package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func TestReadPumpDisconnectsAfterPongTimeout(t *testing.T) {
	disconnected := make(chan struct{})
	connection, client, start := newReadPumpTestClient(t, 8*1024*1024, func(*Client) {
		close(disconnected)
	})
	defer connection.Close()

	client.pongTimeout = 50 * time.Millisecond
	start()

	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("expected client to disconnect after pong timeout")
	}
}

func TestReadPumpExtendsDeadlineWhenPongReceived(t *testing.T) {
	disconnected := make(chan struct{})
	connection, client, start := newReadPumpTestClient(t, 8*1024*1024, func(*Client) {
		close(disconnected)
	})
	defer connection.Close()

	client.pongTimeout = 200 * time.Millisecond
	start()

	time.Sleep(120 * time.Millisecond)
	if err := connection.WriteControl(websocket.PongMessage, []byte("ping"), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("write pong: %v", err)
	}

	select {
	case <-disconnected:
		t.Fatal("client disconnected before the pong-extended deadline")
	case <-time.After(120 * time.Millisecond):
	}

	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("expected client to disconnect after the extended pong deadline")
	}
}

func TestReadPumpAppliesConfiguredMessageLimit(t *testing.T) {
	disconnected := make(chan struct{})
	connection, _, start := newReadPumpTestClient(t, 128, func(*Client) {
		close(disconnected)
	})
	defer connection.Close()
	start()

	if err := connection.WriteJSON(map[string]string{"payload": strings.Repeat("x", 256)}); err != nil {
		t.Fatalf("write oversized message: %v", err)
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("expected oversized message to disconnect the client")
	}
}

func newReadPumpTestClient(t *testing.T, maxMessageBytes int64, onDisconnect func(*Client)) (*websocket.Conn, *Client, func()) {
	t.Helper()

	clientReady := make(chan *Client, 1)
	startReadPump := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Upgrade(writer, request, nil, 1024, 1024)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}

		client, err := NewClient(
			connection,
			uuid.NewString(),
			uuid.NewString(),
			"test-device",
			"test",
			"1.0.0",
			"1.0.0",
			nil,
			maxMessageBytes,
			nil,
			onDisconnect,
		)
		if err != nil {
			t.Errorf("new websocket client: %v", err)
			_ = connection.Close()
			return
		}

		clientReady <- client
		<-startReadPump
		go client.ReadPump()
	}))
	t.Cleanup(server.Close)

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	client := <-clientReady
	return connection, client, func() { close(startReadPump) }
}
