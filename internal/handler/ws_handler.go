package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"colink-server/internal/service"
	"colink-server/internal/ws"
)

type WsHandler struct {
	wsService *service.WsService
	upgrader  websocket.Upgrader
}

func NewWsHandler(wsService *service.WsService) *WsHandler {
	return &WsHandler{
		wsService: wsService,
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
	session, err := h.wsService.ConsumeTicket(c.Query("ticket"))
	if err != nil {
		writeError(c, err)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client, err := ws.NewClient(
		conn,
		session.UserID,
		session.DeviceID,
		session.DeviceName,
		session.DeviceType,
		h.wsService.HandleMessage,
		h.wsService.HandleDisconnect,
	)
	if err != nil {
		_ = conn.Close()
		return
	}

	h.wsService.HandleConnected(client)

	go client.WritePump()
	go client.ReadPump()
}
