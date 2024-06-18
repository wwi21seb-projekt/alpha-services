package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/gin-contrib/graceful"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler/ws"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	pbChat "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
	pbNotification "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"google.golang.org/grpc"
)

var (
	name    = "api-gateway"
	version = "0.1.0"
)

func main() {
	// Initialize logger
	logger := sharedLogging.GetZapLogger()
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			logger.Fatal("Failed to sync logger", zap.Error(err))
		}
	}(logger)

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("failed to load configuration: %v", err)
	}
	logger.Infof("Loaded configuration: %+v", cfg)

	// Create client metrics
	reg := prometheus.NewRegistry()
	clMetrics := tracing.GetClientMetrics()
	reg.MustRegister(clMetrics)

	// Init telemetry
	shutdown, err := tracing.InitTelemetry(context.Background(), name, version)
	if err != nil {
		logger.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			logger.Fatal("Failed to shutdown telemetry", zap.Error(err))
		}
	}()

	// Create dial options
	zapLogger := logger.With(zap.String("service", name)).Desugar()
	dialOpts := tracing.NewClientOptions(clMetrics, zapLogger)

	// Set up a connection to the gRPC post server
	postConn, err := grpc.NewClient(cfg.ServiceEndpoints.PostServiceURL, dialOpts...)
	if err != nil {
		logger.Fatalf("failed to connect to the post-service gRPC server: %v", err)
	}
	defer postConn.Close()

	// Set up a connection to the gRPC user server
	userConn, err := grpc.NewClient(cfg.ServiceEndpoints.UserServiceURL, dialOpts...)
	if err != nil {
		logger.Fatalf("failed to connect to the user-service gRPC server: %v", err)
	}
	defer userConn.Close()

	// Set up a connection to the gRPC chat server
	chatConn, err := grpc.NewClient(cfg.ServiceEndpoints.ChatServiceURL, dialOpts...)
	if err != nil {
		logger.Fatalf("failed to connect to the chat-service gRPC server: %v", err)
	}
	defer chatConn.Close()

	// Set up a connection to the gRPC notification server
	notificationConn, err := grpc.NewClient(cfg.ServiceEndpoints.NotificationServiceURL, dialOpts...)
	if err != nil {
		logger.Fatalf("failed to connect to the notification-service gRPC server: %v", err)
	}
	defer notificationConn.Close()

	// Create client stubs
	userClient := pbUser.NewUserServiceClient(userConn)
	subscriptionClient := pbUser.NewSubscriptionServiceClient(userConn)
	authClient := pbUser.NewAuthenticationServiceClient(userConn)
	chatClient := pbChat.NewChatServiceClient(chatConn)
	postClient := pbPost.NewPostServiceClient(postConn)
	notificationClient := pbNotification.NewNotificationServiceClient(notificationConn)
	pushSubscriptionClient := pbNotification.NewPushServiceClient(notificationConn)

	// Create JWT manager
	jwtManager := manager.NewJWTManager(logger)

	// Create chat hub
	hub := ws.NewHub(logger)

	// Create handler instances
	postHandler := handler.NewPostHandler(postClient)
	userHandler := handler.NewUserHandler(logger, authClient, userClient, subscriptionClient, jwtManager)
	chatHandler := handler.NewChatHandler(logger, jwtManager, chatClient, hub)
	notificationHandler := handler.NewNotificationHandler(logger, notificationClient, pushSubscriptionClient)

	// Expose HTTP endpoint with graceful shutdown
	r, err := graceful.New(gin.New())
	if err != nil {
		logger.Fatal(err)
	}

	// Set up common middleware
	setupCommonMiddleware(r, zapLogger)

	// Expose metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling:     promhttp.ContinueOnError,
		EnableOpenMetrics: true,
	})))

	unauthorizedRouter := r.Group("/api")
	authorizedRouter := r.Group("/api")
	authorizedRouter.Use(middleware.SetClaimsMiddleware(logger, jwtManager))
	setupRoutes(unauthorizedRouter, chatHandler, postHandler, userHandler)
	setupAuthRoutes(authorizedRouter, chatHandler, postHandler, userHandler, notificationHandler)

	// Create a context that listens for termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run chat hub in a separate goroutine
	go hub.Run()

	// Run the gin server
	logger.Info("Starting server...")
	if err = r.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Errorf("Server error: %v", err)
	}

	// Close the chat hub
	hub.Close()
	logger.Info("Server stopped gracefully")
}

func setupCommonMiddleware(r *graceful.Graceful, logger *zap.Logger) {
	r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(logger, true))
	r.Use(otelgin.Middleware("api-gateway"))
	r.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "PATCH", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Accept, Authorization", "Content-Type", "Sec-WebSocket-Protocol"},
		ExposeHeaders: []string{"Content-Length", "Content-Type", "X-Correlation-ID"},
		MaxAge:        12 * time.Hour,
	}))
}

// setupRoutes sets up the routes for the API Gateway
func setupRoutes(apiRouter *gin.RouterGroup, chatHandler handler.ChatHdlr, postHandler handler.PostHdlr, userHandler handler.UserHdlr) {
	// Post routes
	apiRouter.GET("/feed", postHandler.GetFeed)

	// User routes
	apiRouter.POST("/users", middleware.ValidateAndSanitizeStruct(&schema.RegistrationRequest{}), userHandler.RegisterUser)
	apiRouter.POST("/users/login", middleware.ValidateAndSanitizeStruct(&schema.LoginRequest{}), userHandler.LoginUser)
	apiRouter.POST("users/refresh", middleware.ValidateAndSanitizeStruct(&schema.RefreshTokenRequest{}), userHandler.RefreshToken)
	apiRouter.POST("/users/:username/activate", middleware.ValidateAndSanitizeStruct(&schema.ActivationRequest{}), userHandler.ActivateUser)
	apiRouter.DELETE("/users/:username/activate", userHandler.ResendToken)
	apiRouter.POST("/users/:username/reset-password", userHandler.ResetPassword)
	apiRouter.PATCH("/users/:username/reset-password", middleware.ValidateAndSanitizeStruct(&schema.SetPasswordRequest{}), userHandler.SetPassword)

	// Chat routes
	// In theory this is an authorized endpoint as well, but our middleware does not support
	// the workaround we use here, hence we declare it as unauthorized and handle it in the method.
	apiRouter.GET("/chat", chatHandler.Chat)
}

func setupAuthRoutes(authRouter *gin.RouterGroup, chatHandler handler.ChatHdlr, postHandler handler.PostHdlr, userHandler handler.UserHdlr, notificationHandler handler.NotificationHdlr) {
	// Set user routes
	authRouter.GET("/users", userHandler.SearchUsers)
	authRouter.PUT("/users", middleware.ValidateAndSanitizeStruct(&schema.ChangeTrivialInformationRequest{}), userHandler.ChangeTrivialInfo)
	authRouter.PATCH("/users", middleware.ValidateAndSanitizeStruct(&schema.ChangePasswordRequest{}), userHandler.ChangePassword)
	authRouter.GET("/users/:username", userHandler.GetUser)
	authRouter.GET("/users/:username/feed", userHandler.GetUserFeed)
	authRouter.POST("/subscriptions", middleware.ValidateAndSanitizeStruct(&schema.SubscriptionRequest{}), userHandler.CreateSubscription)
	authRouter.DELETE("/subscriptions/:subscriptionId", userHandler.DeleteSubscription)
	authRouter.GET("/subscriptions/:username", userHandler.GetSubscriptions)

	// Set post routes
	authRouter.POST("posts", middleware.ValidateAndSanitizeStruct(&schema.CreatePostRequest{}), postHandler.CreatePost)
	authRouter.GET("/posts", postHandler.QueryPosts)
	authRouter.DELETE("/posts/:postId", postHandler.DeletePost)
	authRouter.POST("/posts/:postId/comments", middleware.ValidateAndSanitizeStruct(&schema.CreateCommentRequest{}), postHandler.CreateComment)
	authRouter.GET("/posts/:postId/comments", postHandler.GetComments)
	authRouter.POST("/posts/:postId/likes", postHandler.CreateLike)
	authRouter.DELETE("/posts/:postId/likes", postHandler.DeleteLike)

	// Set chat routes
	authRouter.GET("/chats", chatHandler.GetChats)
	authRouter.GET("/chats/:chatId", chatHandler.GetChat)
	authRouter.POST("/chats", middleware.ValidateAndSanitizeStruct(&schema.CreateChatRequest{}), chatHandler.CreateChat)

	// Set notification routes
	authRouter.GET("/notifications", notificationHandler.GetNotifications)
	authRouter.DELETE("/notifications/:notificationId", notificationHandler.DeleteNotification)
	authRouter.GET("/push/vapid", notificationHandler.GetPublicKey)
	authRouter.POST("/push/register", notificationHandler.CreatePushSubscription)
}
