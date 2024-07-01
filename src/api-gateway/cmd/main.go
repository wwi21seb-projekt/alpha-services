package main

import (
	"context"
	"errors"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	chatv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/chat/v1"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gin-contrib/graceful"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler/ws"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

var (
	name    = "api-gateway"
	version = "0.1.0"
)

func main() {
	// Initialize logger
	logger, cleanup := sharedLogging.InitializeLogger(name)
	defer cleanup()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("failed to load configuration: %v", err)
	}

	ctx := context.Background()
	// Initialize tracing
	tracingShutdown, err := tracing.InitializeTracing(ctx, name, version)
	if err != nil {
		logger.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer tracingShutdown()

	// Initialize metrics
	metricShutdown, err := metrics.InitializeMetrics(ctx, name, version)
	if err != nil {
		logger.Fatal("Failed to initialize metrics", zap.Error(err))
	}
	defer metricShutdown()

	// Initialize gRPC client connections
	if err = cfg.InitializeClients(logger.Desugar()); err != nil {
		logger.Fatalw("Failed to initialize gRPC clients", "error", err)
	}

	// Create client stubs
	userClient := userv1.NewUserServiceClient(cfg.GRPCClients.UserService)
	subscriptionClient := userv1.NewSubscriptionServiceClient(cfg.GRPCClients.UserService)
	authClient := userv1.NewAuthenticationServiceClient(cfg.GRPCClients.UserService)
	chatClient := chatv1.NewChatServiceClient(cfg.GRPCClients.ChatService)
	postClient := postv1.NewPostServiceClient(cfg.GRPCClients.PostService)
	interactionClient := postv1.NewInteractionServiceClient(cfg.GRPCClients.PostService)
	notificationClient := notificationv1.NewNotificationServiceClient(cfg.GRPCClients.NotificationService)
	pushSubscriptionClient := notificationv1.NewPushServiceClient(cfg.GRPCClients.NotificationService)
	imageClient := imagev1.NewImageServiceClient(cfg.GRPCClients.ImageService)

	// Create JWT manager
	jwtManager := manager.NewJWTManager(logger)

	m := middleware.NewMiddleware(logger, jwtManager)

	// Create chat hub
	hub := ws.NewHub(logger)

	// Create handler instances
	postHandler := handler.NewPostHandler(logger, postClient, jwtManager, interactionClient, *m)
	userHandler := handler.NewUserHandler(logger, authClient, userClient, subscriptionClient, jwtManager)
	chatHandler := handler.NewChatHandler(logger, jwtManager, chatClient, hub)
	notificationHandler := handler.NewNotificationHandler(logger, notificationClient, pushSubscriptionClient)
	imageHandler := handler.NewImageHandler(logger, imageClient)

	// Expose HTTP endpoint with graceful shutdown
	r, err := graceful.New(gin.New())
	if err != nil {
		logger.Fatal(err)
	}

	// Set up common middleware
	setupCommonMiddleware(r, logger.Desugar())

	unauthorizedRouter := r.Group("/api")
	authorizedRouter := r.Group("/api")
	authorizedRouter.Use(m.SetClaimsMiddleware())
	setupRoutes(unauthorizedRouter, m, chatHandler, postHandler, userHandler, imageHandler)
	setupAuthRoutes(authorizedRouter, m, chatHandler, postHandler, userHandler, notificationHandler)

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
func setupRoutes(r *gin.RouterGroup, m *middleware.Middleware, ch handler.ChatHdlr, ph handler.PostHdlr, uh handler.UserHdlr, ih handler.ImageHdlr) {
	r.GET("/imprint", HandleImprint)

	// Post routes
	r.GET("/feed", ph.GetFeed)

	// Image routes
	r.GET("/images", ih.GetImage)

	// User routes
	r.POST("/users", m.ValidateAndSanitizeStruct(dto.RegistrationRequest{}), uh.RegisterUser)
	r.POST("/users/login", m.ValidateAndSanitizeStruct(dto.LoginRequest{}), uh.LoginUser)
	r.POST("users/refresh", m.ValidateAndSanitizeStruct(schema.RefreshTokenRequest{}), uh.RefreshToken)
	r.POST("/users/:username/activate", m.ValidateAndSanitizeStruct(schema.ActivationRequest{}), uh.ActivateUser)
	r.DELETE("/users/:username/activate", uh.ResendToken)
	r.POST("/users/:username/reset-password", uh.ResetPassword)
	r.PATCH("/users/:username/reset-password", m.ValidateAndSanitizeStruct(schema.SetPasswordRequest{}), uh.SetPassword)

	// Chat routes
	// In theory this is an authorized endpoint as well, but our middleware does not support
	// the workaround we use here, hence we declare it as unauthorized and handle it in the method.
	r.GET("/chat", ch.Chat)
}

func setupAuthRoutes(r *gin.RouterGroup, m *middleware.Middleware, ch handler.ChatHdlr, ph handler.PostHdlr, uh handler.UserHdlr, nh handler.NotificationHdlr) {
	// Set user routes
	r.GET("/users", uh.SearchUsers)
	r.PUT("/users", m.ValidateAndSanitizeStruct(schema.ChangeTrivialInformationRequest{}), uh.ChangeTrivialInfo)
	r.PATCH("/users", m.ValidateAndSanitizeStruct(schema.ChangePasswordRequest{}), uh.ChangePassword)
	r.GET("/users/:username", uh.GetUser)
	r.GET("/users/:username/feed", ph.GetUserFeed)
	r.POST("/subscriptions", m.ValidateAndSanitizeStruct(schema.SubscriptionRequest{}), uh.CreateSubscription)
	r.DELETE("/subscriptions/:subscriptionId", uh.DeleteSubscription)
	r.GET("/subscriptions/:username", uh.GetSubscriptions)

	// Set post routes
	r.POST("posts", m.ValidateAndSanitizeStruct(dto.CreatePostRequest{}), ph.CreatePost)
	r.GET("/posts", ph.QueryPosts)
	r.DELETE("/posts/:postId", ph.DeletePost)
	r.POST("/posts/:postId/comments", m.ValidateAndSanitizeStruct(dto.CreateCommentRequest{}), ph.CreateComment)
	r.GET("/posts/:postId/comments", ph.GetComments)
	r.POST("/posts/:postId/likes", ph.CreateLike)
	r.DELETE("/posts/:postId/likes", ph.DeleteLike)

	// Set chat routes
	r.GET("/chats", ch.GetChats)
	r.GET("/chats/:chatId", ch.GetChat)
	r.POST("/chats", m.ValidateAndSanitizeStruct(schema.CreateChatRequest{}), ch.CreateChat)

	// Set notification routes
	r.GET("/notifications", nh.GetNotifications)
	r.DELETE("/notifications/:notificationId", nh.DeleteNotification)
	r.GET("/push/vapid", nh.GetPublicKey)
	r.POST("/push/register", m.ValidateAndSanitizeStruct(schema.CreatePushSubscriptionRequest{}), nh.CreatePushSubscription)
}

func HandleImprint(c *gin.Context) {
	c.JSON(200, gin.H{
		"text": "This is the imprint",
	})
}
