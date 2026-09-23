package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Opaque tokens are base64url-encoded JSON payloads. They are opaque to
// clients, which must return them unchanged.

const tokenKindList = "list"      // GET /notes page token
const tokenKindPage = "page"      // sync snapshot page token
const tokenKindCursor = "cursor"  // sync change cursor

type opaqueToken struct {
	Kind         string `json:"k"`
	UserID       string `json:"userId,omitempty"`
	Seq          int64  `json:"seq,omitempty"`
	Phase        string `json:"phase,omitempty"`
	LastSeq      int64  `json:"lastSeq,omitempty"`
	LastID       string `json:"lastId,omitempty"`
	UpdatedAt    string `json:"u,omitempty"`
	ID           string `json:"id,omitempty"`
	TagID        string `json:"tagId,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func encodeOpaqueToken(payload opaqueToken) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeOpaqueToken(raw string) (*opaqueToken, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	var payload opaqueToken
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	return &payload, nil
}
