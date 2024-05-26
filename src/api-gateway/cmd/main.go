package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	"github.com/gin-contrib/graceful"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set up a connection to the gRPC post server
	postConn, err := grpc.NewClient(cfg.PostServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer postConn.Close()

	// Set up a connection to the gRPC user server
	userConn, err := grpc.NewClient(cfg.UserServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer userConn.Close()

	// Create client stubs
	postClient := pbPost.NewPostServiceClient(postConn)
	userClient := pbUser.NewProfileServiceClient(userConn)
	subscriptionClient := pbUser.NewSubscriptionServiceClient(postConn)
	authClient := pbUser.NewAuthenticationServiceClient(userConn)

	// Create handler instances
	postHandler := handler.NewPostHandler(postClient)
	userHandler := handler.NewUserHandler(authClient, userClient, subscriptionClient)

	// Expose HTTP endpoint with graceful shutdown
	r, err := graceful.Default()
	if err != nil {
		log.Fatal(err)
	}

	setupRoutes(r, postHandler)

	// Create a context that listens for termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("Starting server...")
	if err = r.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("Server error: %v", err)
	}

	log.Info("Server stopped gracefully")
}

// setupRoutes sets up the routes for the API Gateway
func setupRoutes(r *graceful.Graceful, postHandler *handler.PostHandler) {
	apiRouter := r.Group("/api")

	// Set public routes
	apiRouter.GET("/feed", postHandler.GetFeed)

	// Set authenticated routes
	apiRouter.POST("posts", middleware.ValidateAndSanitizeStruct(&schema.CreatePostRequest{}), postHandler.CreatePost)
}

func setupAuthRoutes(r *graceful.Graceful, userHandler *handler.UserHandler, postHandler *handler.PostHandler) {
	authRouter := r.Group("/api")
	authRouter.Use(middleware.RequireAuthMiddleware())
	authRouter.Use(middleware.SetClaimsMiddleware())

	// Set public routes
	authRouter.POST("/posts", userHandler.RegisterUser)
	authRouter.POST("/posts/login", userHandler.LoginUser)
}
