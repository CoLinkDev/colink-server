package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
	"colink-server/internal/ws"
)

type TicketResult struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int64  `json:"expiresIn"`
}

type WsSession struct {
	UserID     string
	DeviceID   string
	DeviceName string
	DeviceType string
}

type WsService struct {
	deviceRepo      *repository.DeviceRepository
	ticketRepo      *repository.TicketRepository
	hub             *ws.Hub
	ticketTTL       time.Duration
	ticketLimitMu   sync.Mutex
	ticketLimitByID map[string][]time.Time
}

func NewWsService(
	deviceRepo *repository.DeviceRepository,
	ticketRepo *repository.TicketRepository,
	hub *ws.Hub,
	ticketTTL time.Duration,
) *WsService {
	return &WsService{
		deviceRepo:      deviceRepo,
		ticketRepo:      ticketRepo,
		hub:             hub,
		ticketTTL:       ticketTTL,
		ticketLimitByID: make(map[string][]time.Time),
	}
}

func (s *WsService) IssueTicket(userID string, deviceID string) (*TicketResult, error) {
	device, err := s.ensureOwnedDevice(userID, deviceID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if !s.allowTicketIssue(userID, now) {
		return nil, pkg.NewAppError(http.StatusTooManyRequests, pkg.CodeRateLimited, "rate limited")
	}
	if err := s.ticketRepo.Cleanup(now); err != nil {
		return nil, pkg.InternalError(err)
	}

	ticketValue, err := pkg.GenerateOpaqueToken(48)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	record := &model.WsTicket{
		UserID:    device.UserID,
		DeviceID:  device.ID,
		Ticket:    ticketValue,
		ExpiresAt: now.Add(s.ticketTTL),
	}
	if err := s.ticketRepo.Create(record); err != nil {
		return nil, pkg.InternalError(err)
	}

	return &TicketResult{
		Ticket:    ticketValue,
		ExpiresIn: int64(s.ticketTTL / time.Second),
	}, nil
}

func (s *WsService) ConsumeTicket(ticket string) (*WsSession, error) {
	if ticket == "" {
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
	}

	record, err := s.ticketRepo.ConsumeValid(ticket, time.Now().UTC())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	device, err := s.deviceRepo.FindByIDAndUserID(record.DeviceID, record.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	return &WsSession{
		UserID:     record.UserID.String(),
		DeviceID:   record.DeviceID.String(),
		DeviceName: device.Name,
		DeviceType: device.Type,
	}, nil
}

func (s *WsService) HandleConnected(client *ws.Client) {
	if s.hub.Register(client) {
		s.broadcastOnline(client)
	}
}

func (s *WsService) HandleDisconnect(client *ws.Client) {
	if !s.hub.Unregister(client) {
		return
	}

	_ = s.deviceRepo.UpdateLastSeen(client.DeviceUUID(), time.Now().UTC())
	s.broadcastOffline(client)
}

func (s *WsService) HandleMessage(client *ws.Client, message ws.ClientMessage) {
	switch message.Type {
	case "ping":
		client.Send(ws.MessageEnvelope{
			ID:        message.ID,
			Type:      "pong",
			Timestamp: time.Now().UTC().UnixMilli(),
		})
	case "relay":
		s.handleRelay(client, message)
	case "announce":
		s.handleAnnounce(client, message)
	}
}

func (s *WsService) ensureOwnedDevice(userID string, deviceID string) (*model.Device, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return nil, err
	}

	device, err := s.deviceRepo.FindByIDAndUserID(deviceUUID, userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusNotFound, pkg.CodeDeviceNotFound, "device not found")
		}
		return nil, pkg.InternalError(err)
	}

	return device, nil
}

func (s *WsService) allowTicketIssue(userID string, now time.Time) bool {
	s.ticketLimitMu.Lock()
	defer s.ticketLimitMu.Unlock()

	windowStart := now.Add(-time.Minute)
	history := s.ticketLimitByID[userID]
	filtered := make([]time.Time, 0, len(history))
	for _, item := range history {
		if item.After(windowStart) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) >= 5 {
		s.ticketLimitByID[userID] = filtered
		return false
	}

	filtered = append(filtered, now)
	s.ticketLimitByID[userID] = filtered
	return true
}

func (s *WsService) handleRelay(client *ws.Client, message ws.ClientMessage) {
	if message.To == nil {
		return
	}
	if _, err := parseUUID(*message.To); err != nil {
		return
	}

	from := client.DeviceID()
	to := *message.To
	s.hub.SendToDevice(client.UserID(), to, ws.MessageEnvelope{
		ID:        message.ID,
		Type:      "relay",
		From:      &from,
		To:        &to,
		Payload:   json.RawMessage(message.Payload),
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}

func (s *WsService) handleAnnounce(client *ws.Client, message ws.ClientMessage) {
	var payload ws.AnnouncePayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return
	}
	if payload.LocalIP == "" || payload.LocalPort <= 0 {
		return
	}

	client.UpdateAnnouncement(payload.LocalIP, payload.LocalPort)
	s.broadcastOnline(client)
}

func (s *WsService) broadcastOnline(client *ws.Client) {
	from := client.DeviceID()
	localIP, localPort := client.Announcement()
	s.hub.Broadcast(client.UserID(), client.DeviceID(), ws.MessageEnvelope{
		ID:   pkg.NewMessageID(),
		Type: "device.online",
		From: &from,
		Payload: ws.DeviceOnlinePayload{
			Name:      client.DeviceName(),
			Type:      client.DeviceType(),
			LocalIP:   localIP,
			LocalPort: localPort,
		},
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}

func (s *WsService) broadcastOffline(client *ws.Client) {
	from := client.DeviceID()
	s.hub.Broadcast(client.UserID(), client.DeviceID(), ws.MessageEnvelope{
		ID:        pkg.NewMessageID(),
		Type:      "device.offline",
		From:      &from,
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}
