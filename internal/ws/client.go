package ws

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn           *websocket.Conn
	userID         string
	deviceID       string
	deviceUUID     uuid.UUID
	deviceName     string
	deviceType     string
	send           chan any
	process        func(*Client, ClientMessage)
	onDisconnect   func(*Client)
	closeOnce      sync.Once
	disconnectOnce sync.Once
	stateMu        sync.RWMutex
	closed         bool
	metaMu         sync.RWMutex
	localIP        string
	localPort      int
}

func NewClient(
	conn *websocket.Conn,
	userID string,
	deviceID string,
	deviceName string,
	deviceType string,
	process func(*Client, ClientMessage),
	onDisconnect func(*Client),
) (*Client, error) {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:         conn,
		userID:       userID,
		deviceID:     deviceID,
		deviceUUID:   deviceUUID,
		deviceName:   deviceName,
		deviceType:   deviceType,
		send:         make(chan any, 32),
		process:      process,
		onDisconnect: onDisconnect,
	}, nil
}

func (c *Client) ReadPump() {
	defer c.handleDisconnect()
	c.conn.SetReadLimit(1024 * 1024)

	for {
		var message ClientMessage
		if err := c.conn.ReadJSON(&message); err != nil {
			return
		}

		if c.process != nil {
			c.process(c, message)
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	defer c.handleDisconnect()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}
}

func (c *Client) Send(message any) {
	c.stateMu.RLock()
	if c.closed {
		c.stateMu.RUnlock()
		return
	}
	c.stateMu.RUnlock()

	select {
	case c.send <- message:
	default:
		c.Close()
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.stateMu.Lock()
		c.closed = true
		c.stateMu.Unlock()
		close(c.send)
		_ = c.conn.Close()
	})
}

func (c *Client) UserID() string {
	return c.userID
}

func (c *Client) DeviceID() string {
	return c.deviceID
}

func (c *Client) DeviceUUID() uuid.UUID {
	return c.deviceUUID
}

func (c *Client) DeviceName() string {
	return c.deviceName
}

func (c *Client) DeviceType() string {
	return c.deviceType
}

func (c *Client) UpdateAnnouncement(localIP string, localPort int) {
	c.metaMu.Lock()
	defer c.metaMu.Unlock()

	c.localIP = localIP
	c.localPort = localPort
}

func (c *Client) Announcement() (string, int) {
	c.metaMu.RLock()
	defer c.metaMu.RUnlock()

	return c.localIP, c.localPort
}

func (c *Client) handleDisconnect() {
	c.disconnectOnce.Do(func() {
		c.Close()
		if c.onDisconnect != nil {
			c.onDisconnect(c)
		}
	})
}
