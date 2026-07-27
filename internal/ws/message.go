package ws

import "encoding/json"

type ClientMessage struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	To            *string         `json:"to,omitempty"`
	CorrelationID *string         `json:"correlationId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type MessageEnvelope struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	From          *string `json:"from,omitempty"`
	To            *string `json:"to,omitempty"`
	CorrelationID *string `json:"correlationId,omitempty"`
	Payload       any     `json:"payload,omitempty"`
	Timestamp     int64   `json:"timestamp,omitempty"`
}

type DeviceOnlinePayload struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	BusinessVersion string `json:"businessVersion"`
	WsVersion       *string `json:"wsVersion,omitempty"`
}

type PushNotificationPayload struct {
	Title      *string `json:"title,omitempty"`
	Subtitle   *string `json:"subtitle,omitempty"`
	Body       *string `json:"body,omitempty"`
	Markdown   *string `json:"markdown,omitempty"`
	Level      *string `json:"level,omitempty"`
	Volume     *int    `json:"volume,omitempty"`
	Badge      *int    `json:"badge,omitempty"`
	Sound      *string `json:"sound,omitempty"`
	Icon       *string `json:"icon,omitempty"`
	Image      *string `json:"image,omitempty"`
	Group      *string `json:"group,omitempty"`
	URL        *string `json:"url,omitempty"`
	Copy       *string `json:"copy,omitempty"`
	AutoCopy   *bool   `json:"autoCopy,omitempty"`
	Call       *bool   `json:"call,omitempty"`
	IsArchive  *bool   `json:"isArchive,omitempty"`
	TTL        *int    `json:"ttl,omitempty"`
	PushID     *string `json:"id,omitempty"`
	Delete     *bool   `json:"delete,omitempty"`
	Action     *string `json:"action,omitempty"`
	Ciphertext *string `json:"ciphertext,omitempty"`
}

type PushEnvelope struct {
	ID            string                  `json:"id"`
	Type          string                  `json:"type"`
	From          *string                 `json:"from"`
	To            string                  `json:"to"`
	CorrelationID *string                 `json:"correlationId"`
	Payload       PushNotificationPayload `json:"payload"`
	Timestamp     int64                   `json:"timestamp"`
}
