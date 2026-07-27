package ws

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn            *websocket.Conn
	userID          string
	deviceID        string
	deviceUUID      uuid.UUID
	deviceName      string
	deviceType      string
	businessVersion string
	wsVersion       string
	advertisedWsVersion *string
	send            chan any
	process         func(*Client, ClientMessage)
	onDisconnect    func(*Client)
	closeOnce       sync.Once
	disconnectOnce  sync.Once
	stateMu         sync.RWMutex
	closed          bool
}

func NewClient(
	conn *websocket.Conn,
	userID string,
	deviceID string,
	deviceName string,
	deviceType string,
	businessVersion string,
	wsVersion string,
	advertisedWsVersion *string,
	process func(*Client, ClientMessage),
	onDisconnect func(*Client),
) (*Client, error) {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:            conn,
		userID:          userID,
		deviceID:        deviceID,
		deviceUUID:      deviceUUID,
		deviceName:      deviceName,
		deviceType:      deviceType,
		businessVersion: businessVersion,
		wsVersion:       wsVersion,
		advertisedWsVersion: advertisedWsVersion,
		send:            make(chan any, 32),
		process:         process,
		onDisconnect:    onDisconnect,
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

func (c *Client) Send(message any) bool {
	c.stateMu.RLock()
	if c.closed {
		c.stateMu.RUnlock()
		return false
	}

	select {
	case c.send <- message:
		c.stateMu.RUnlock()
		return true
	default:
		c.stateMu.RUnlock()
		c.Close()
		return false
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

func (c *Client) BusinessVersion() string {
	return c.businessVersion
}

func (c *Client) WsVersion() string {
	return c.wsVersion
}

func (c *Client) AdvertisedWsVersion() *string {
	return c.advertisedWsVersion
}

func (c *Client) SupportsPushNotifications() bool {
	version, ok := ParseSemver(c.wsVersion)
	return ok && version.Major == 1 && (version.Minor > 1 || version.Minor == 1 && version.Patch >= 0)
}

func (c *Client) handleDisconnect() {
	c.disconnectOnce.Do(func() {
		c.Close()
		if c.onDisconnect != nil {
			c.onDisconnect(c)
		}
	})
}
