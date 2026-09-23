package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"colink-server/internal/config"
	"colink-server/internal/middleware"
	"colink-server/internal/repository"
	"colink-server/internal/service"
	"colink-server/internal/ws"
)

func NewMainRouter(cfg *config.Config, db *gorm.DB, log *zap.Logger) *gin.Engine {
	userRepo := repository.NewUserRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	noteRepo := repository.NewNoteRepository(db)
	noteTagRepo := repository.NewNoteTagRepository(db)
	noteAttachmentRepo := repository.NewNoteAttachmentRepository(db)
	noteChangeLogRepo := repository.NewNoteChangeLogRepository(db)
	hub := ws.NewHub()

	authService := service.NewAuthService(
		db,
		userRepo,
		tokenRepo,
		cfg.JWT.Secret,
		cfg.JWT.AccessTTL,
		cfg.JWT.RefreshTTL,
	)
	deviceService := service.NewDeviceService(deviceRepo, hub, cfg.Device.Limit)
	wsService := service.NewWsService(deviceRepo, ticketRepo, hub, cfg.WS.TicketTTL, cfg.WS.TicketRateLimit, log)
	noteService := service.NewNoteService(db, noteRepo, noteTagRepo, noteAttachmentRepo, noteChangeLogRepo, cfg.Notes)
	tagService := service.NewTagService(db, noteTagRepo, noteRepo, noteChangeLogRepo)
	attachmentService := service.NewAttachmentService(db, noteAttachmentRepo, noteRepo, cfg.Notes)
	syncService := service.NewSyncService(db, noteRepo, noteTagRepo, noteAttachmentRepo, noteChangeLogRepo)

	authHandler := NewAuthHandler(authService)
	deviceHandler := NewDeviceHandler(deviceService)
	meHandler := NewMeHandler(authService)
	wsHandler := NewWsHandler(wsService, cfg.WS.MaxMessageBytes)
	pushHandler := NewPushHandler(wsService)
	noteHandler := NewNoteHandler(noteService)
	noteTagHandler := NewNoteTagHandler(tagService)
	noteAttachmentHandler := NewNoteAttachmentHandler(attachmentService)
	syncHandler := NewSyncHandler(syncService)
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret, userRepo)

	router := newBaseRouter(log)
	router.Use(requestBodyLimit(cfg))
	registerMainRoutes(router, authHandler, deviceHandler, meHandler, wsHandler, pushHandler, noteHandler, noteTagHandler, noteAttachmentHandler, syncHandler, authMiddleware)
	serveFrontend(router)
	return router
}

func NewUpdateRouter(cfg *config.Config, db *gorm.DB, log *zap.Logger) (*gin.Engine, *service.UpdateService) {
	releaseRepo := repository.NewReleaseRepository(db)
	updateService := service.NewUpdateService(releaseRepo, cfg.Update, log)
	updateHandler := NewUpdateHandler(updateService)

	router := newBaseRouter(log)
	registerUpdateRoutes(router, updateHandler)
	return router, updateService
}

func NewRouter(cfg *config.Config, db *gorm.DB, log *zap.Logger) (*gin.Engine, *service.UpdateService) {
	router := newBaseRouter(log)
	router.Use(requestBodyLimit(cfg))

	userRepo := repository.NewUserRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	releaseRepo := repository.NewReleaseRepository(db)
	noteRepo := repository.NewNoteRepository(db)
	noteTagRepo := repository.NewNoteTagRepository(db)
	noteAttachmentRepo := repository.NewNoteAttachmentRepository(db)
	noteChangeLogRepo := repository.NewNoteChangeLogRepository(db)
	hub := ws.NewHub()

	authService := service.NewAuthService(
		db,
		userRepo,
		tokenRepo,
		cfg.JWT.Secret,
		cfg.JWT.AccessTTL,
		cfg.JWT.RefreshTTL,
	)
	deviceService := service.NewDeviceService(deviceRepo, hub, cfg.Device.Limit)
	wsService := service.NewWsService(deviceRepo, ticketRepo, hub, cfg.WS.TicketTTL, cfg.WS.TicketRateLimit, log)
	updateService := service.NewUpdateService(releaseRepo, cfg.Update, log)
	noteService := service.NewNoteService(db, noteRepo, noteTagRepo, noteAttachmentRepo, noteChangeLogRepo, cfg.Notes)
	tagService := service.NewTagService(db, noteTagRepo, noteRepo, noteChangeLogRepo)
	attachmentService := service.NewAttachmentService(db, noteAttachmentRepo, noteRepo, cfg.Notes)
	syncService := service.NewSyncService(db, noteRepo, noteTagRepo, noteAttachmentRepo, noteChangeLogRepo)

	authHandler := NewAuthHandler(authService)
	deviceHandler := NewDeviceHandler(deviceService)
	meHandler := NewMeHandler(authService)
	wsHandler := NewWsHandler(wsService, cfg.WS.MaxMessageBytes)
	pushHandler := NewPushHandler(wsService)
	updateHandler := NewUpdateHandler(updateService)
	noteHandler := NewNoteHandler(noteService)
	noteTagHandler := NewNoteTagHandler(tagService)
	noteAttachmentHandler := NewNoteAttachmentHandler(attachmentService)
	syncHandler := NewSyncHandler(syncService)
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret, userRepo)

	registerMainRoutes(router, authHandler, deviceHandler, meHandler, wsHandler, pushHandler, noteHandler, noteTagHandler, noteAttachmentHandler, syncHandler, authMiddleware)
	registerUpdateRoutes(router, updateHandler)
	serveFrontend(router)
	return router, updateService
}

func newBaseRouter(log *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger(log))
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(204)
	})
	return router
}

func requestBodyLimit(cfg *config.Config) gin.HandlerFunc {
	const defaultLimit = int64(4 << 20)
	const requestOverhead = int64(1 << 20)

	jsonLimit := cfg.Notes.MaxMarkdownBytes + requestOverhead
	if jsonLimit < defaultLimit || jsonLimit < cfg.Notes.MaxMarkdownBytes {
		jsonLimit = defaultLimit
	}
	pushLimit := cfg.WS.MaxMessageBytes + requestOverhead
	if pushLimit < cfg.WS.MaxMessageBytes {
		pushLimit = int64(^uint64(0) >> 1)
	}
	if jsonLimit < pushLimit {
		jsonLimit = pushLimit
	}
	uploadLimit := cfg.Notes.MaxAttachmentBytes + requestOverhead
	if uploadLimit < cfg.Notes.MaxAttachmentBytes {
		uploadLimit = int64(^uint64(0) >> 1)
	}

	return func(c *gin.Context) {
		if c.Request.Body != nil {
			limit := jsonLimit
			if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/v1/note-attachments" {
				limit = uploadLimit
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

func registerMainRoutes(
	router *gin.Engine,
	authHandler *AuthHandler,
	deviceHandler *DeviceHandler,
	meHandler *MeHandler,
	wsHandler *WsHandler,
	pushHandler *PushHandler,
	noteHandler *NoteHandler,
	noteTagHandler *NoteTagHandler,
	noteAttachmentHandler *NoteAttachmentHandler,
	syncHandler *SyncHandler,
	authMiddleware *middleware.AuthMiddleware,
) {
	api := router.Group("/api")
	v1 := api.Group("/v1")

	auth := v1.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.Refresh)
	auth.POST("/logout", authMiddleware.RequireAuth(), authHandler.Logout)
	auth.POST("/change-password", authMiddleware.RequireAuth(), authHandler.ChangePassword)

	v1.GET("/me", authMiddleware.RequireAuth(), meHandler.Get)
	v1.PUT("/me/username", authMiddleware.RequireAuth(), meHandler.UpdateUsername)

	devices := v1.Group("/devices")
	devices.Use(authMiddleware.RequireAuth())
	devices.POST("", deviceHandler.Register)
	devices.GET("", deviceHandler.List)
	devices.PUT("/:deviceId", deviceHandler.Update)
	devices.DELETE("/:deviceId", deviceHandler.Delete)
	devices.PUT("/:deviceId/key", deviceHandler.RotateKey)

	wsGroup := v1.Group("/ws")
	wsGroup.Use(authMiddleware.RequireAuth())
	wsGroup.POST("/ticket", wsHandler.CreateTicket)

	push := api.Group("/push")
	push.Use(authMiddleware.RequireAuthWithFailure(pushHandler.Unauthorized))
	push.GET("", pushHandler.Send)
	push.POST("", pushHandler.Send)
	push.GET("/*path", pushHandler.Send)
	push.POST("/*path", pushHandler.Send)

	notes := v1.Group("/notes")
	notes.Use(authMiddleware.RequireAuth())
	notes.GET("", noteHandler.List)
	notes.POST("", noteHandler.Create)
	notes.GET("/sync/snapshot", syncHandler.Snapshot)
	notes.GET("/sync/changes", syncHandler.Changes)
	notes.GET("/storage", noteAttachmentHandler.Storage)
	notes.GET("/:noteId", noteHandler.Get)
	notes.PUT("/:noteId", noteHandler.Update)
	notes.DELETE("/:noteId", noteHandler.Delete)

	noteTags := v1.Group("/note-tags")
	noteTags.Use(authMiddleware.RequireAuth())
	noteTags.GET("", noteTagHandler.List)
	noteTags.POST("", noteTagHandler.Create)
	noteTags.PUT("/:tagId", noteTagHandler.Update)
	noteTags.DELETE("/:tagId", noteTagHandler.Delete)

	noteAttachments := v1.Group("/note-attachments")
	noteAttachments.Use(authMiddleware.RequireAuth())
	noteAttachments.POST("", noteAttachmentHandler.Upload)
	noteAttachments.GET("/:attachmentId", noteAttachmentHandler.Get)
	noteAttachments.GET("/:attachmentId/content", noteAttachmentHandler.Content)
	noteAttachments.GET("/:attachmentId/references", noteAttachmentHandler.References)
	noteAttachments.DELETE("/:attachmentId", noteAttachmentHandler.Delete)

	router.GET("/ws/v1", wsHandler.Connect)
}

func registerUpdateRoutes(router *gin.Engine, updateHandler *UpdateHandler) {
	api := router.Group("/api")
	v1 := api.Group("/v1")

	update := v1.Group("/update")
	update.GET("/check", updateHandler.CheckUpdate)
	update.GET("/tauri/:target/:arch/:currentVersion", updateHandler.GetTauriManifest)
	update.GET("/download/:platform/:version/:filename", updateHandler.DownloadAsset)
}
