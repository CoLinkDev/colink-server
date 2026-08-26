package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"colink-server/internal/service"
	"colink-server/internal/ws"
)

type WsHandler struct {
	wsService      *service.WsService
	maxMessageBytes int64
	upgrader       websocket.Upgrader
}

func NewWsHandler(wsService *service.WsService, maxMessageBytes int64) *WsHandler {
	return &WsHandler{
		wsService:       wsService,
		maxMessageBytes: maxMessageBytes,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *WsHandler) CreateTicket(c *gin.Context) {
	var req struct {
		DeviceID string `json:"deviceId" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	result, err := h.wsService.IssueTicket(userIDFromContext(c), req.DeviceID)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *WsHandler) Connect(c *gin.Context) {
	businessVersion := c.Query("businessVersion")
	if err := h.wsService.ValidateBusinessVersion(businessVersion); err != nil {
		writeError(c, err)
		return
	}
	advertisedWsVersion, hasWsVersion := c.GetQuery("wsVersion")
	wsVersion, err := h.wsService.ValidateCloudWebSocketVersion(advertisedWsVersion)
	if err != nil {
		writeError(c, err)
		return
	}
	var reportedWsVersion *string
	if hasWsVersion {
		reportedWsVersion = &advertisedWsVersion
	}

	session, err := h.wsService.ConsumeTicket(c.Query("ticket"))
	if err != nil {
		writeError(c, err)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.wsService.LogUpgradeFailure(err)
		return
	}

	client, err := ws.NewClient(
		conn,
		session.UserID,
		session.DeviceID,
		session.DeviceName,
		session.DeviceType,
		businessVersion,
		wsVersion,
		reportedWsVersion,
		h.maxMessageBytes,
		h.wsService.HandleMessage,
		h.wsService.HandleDisconnect,
	)
	if err != nil {
		h.wsService.LogClientCreationFailure(err)
		_ = conn.Close()
		return
	}

	h.wsService.HandleConnected(client)

	go client.WritePump()
	go client.ReadPump()
}
