package ws

import "sync"

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[string]*Client),
	}
}

func (h *Hub) Register(client *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	userClients, ok := h.clients[client.UserID()]
	if !ok {
		userClients = make(map[string]*Client)
		h.clients[client.UserID()] = userClients
	}

	_, existed := userClients[client.DeviceID()]
	if existed {
		userClients[client.DeviceID()].Close()
	}
	userClients[client.DeviceID()] = client
	return !existed
}

func (h *Hub) Unregister(client *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	userClients, ok := h.clients[client.UserID()]
	if !ok {
		return false
	}

	current, ok := userClients[client.DeviceID()]
	if !ok || current != client {
		return false
	}

	delete(userClients, client.DeviceID())
	if len(userClients) == 0 {
		delete(h.clients, client.UserID())
	}

	return true
}

func (h *Hub) Broadcast(userID string, excludeDeviceID string, message any) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for deviceID, client := range h.clients[userID] {
		if deviceID == excludeDeviceID {
			continue
		}
		client.Send(message)
	}
}

func (h *Hub) SendToDevice(userID string, deviceID string, message any) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	client, ok := h.clients[userID][deviceID]
	if !ok {
		return false
	}

	client.Send(message)
	return true
}

func (h *Hub) IsOnline(userID string, deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, ok := h.clients[userID][deviceID]
	return ok
}

func (h *Hub) Disconnect(userID string, deviceID string) {
	h.mu.RLock()
	client := h.clients[userID][deviceID]
	h.mu.RUnlock()

	if client != nil {
		client.Close()
	}
}
