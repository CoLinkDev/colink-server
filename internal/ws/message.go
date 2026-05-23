package ws

import "encoding/json"

type ClientMessage struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	To      *string         `json:"to,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type MessageEnvelope struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	From      *string `json:"from,omitempty"`
	To        *string `json:"to,omitempty"`
	Payload   any     `json:"payload,omitempty"`
	Timestamp int64   `json:"timestamp,omitempty"`
}

type AnnouncePayload struct {
	LocalIP   string `json:"localIp"`
	LocalPort int    `json:"localPort"`
}

type DeviceOnlinePayload struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	LocalIP   string `json:"localIp,omitempty"`
	LocalPort int    `json:"localPort,omitempty"`
}
