package handler

import (
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
	hub := ws.NewHub()

	authService := service.NewAuthService(
		db,
		userRepo,
		tokenRepo,
		cfg.JWT.Secret,
		cfg.JWT.AccessTTL,
		cfg.JWT.RefreshTTL,
	)
	deviceService := service.NewDeviceService(deviceRepo, hub)
	wsService := service.NewWsService(deviceRepo, ticketRepo, hub, cfg.WS.TicketTTL)

	authHandler := NewAuthHandler(authService)
	deviceHandler := NewDeviceHandler(deviceService)
	meHandler := NewMeHandler(authService)
	wsHandler := NewWsHandler(wsService)
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)

	router := newBaseRouter(log)
	registerMainRoutes(router, authHandler, deviceHandler, meHandler, wsHandler, authMiddleware)
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

	userRepo := repository.NewUserRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	releaseRepo := repository.NewReleaseRepository(db)
	hub := ws.NewHub()

	authService := service.NewAuthService(
		db,
		userRepo,
		tokenRepo,
		cfg.JWT.Secret,
		cfg.JWT.AccessTTL,
		cfg.JWT.RefreshTTL,
	)
	deviceService := service.NewDeviceService(deviceRepo, hub)
	wsService := service.NewWsService(deviceRepo, ticketRepo, hub, cfg.WS.TicketTTL)
	updateService := service.NewUpdateService(releaseRepo, cfg.Update, log)

	authHandler := NewAuthHandler(authService)
	deviceHandler := NewDeviceHandler(deviceService)
	meHandler := NewMeHandler(authService)
	wsHandler := NewWsHandler(wsService)
	updateHandler := NewUpdateHandler(updateService)
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)

	registerMainRoutes(router, authHandler, deviceHandler, meHandler, wsHandler, authMiddleware)
	registerUpdateRoutes(router, updateHandler)
	serveFrontend(router)
	return router, updateService
}

func newBaseRouter(log *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.Logger(log))
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(204)
	})
	return router
}

func registerMainRoutes(
	router *gin.Engine,
	authHandler *AuthHandler,
	deviceHandler *DeviceHandler,
	meHandler *MeHandler,
	wsHandler *WsHandler,
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
