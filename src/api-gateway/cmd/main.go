package main

import (
	"context"
	"errors"
	"flag"
	"github.com/gin-contrib/graceful"
	"github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	"os/signal"
	"syscall"
)

var (
	postGrpcURL = *flag.String("POST_ADDR", os.Getenv("POST_ADDR"), "URL of the gRPC server for posts")
)

func main() {
	// Parse the command-line flags
	flag.Parse()

	// Set up a connection to the gRPC server
	postConn, err := grpc.NewClient(postGrpcURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logrus.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer postConn.Close()

	// Create client stubs
	postClientStub := pbPost.NewPostServiceClient(postConn)

	// Create handler instances
	postHandler := handler.NewPostHandler(postClientStub)

	// Expose HTTP endpoint with go-micro server
	r, err := graceful.Default()
	if err != nil {
		logrus.Fatal(err)
	}

	setupRoutes(r, postHandler)

	// Create a context that listens for termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err = r.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logrus.Info(err)
	}
}

// setupRoutes sets up the routes for the API Gateway
func setupRoutes(r *graceful.Graceful, postHandler *handler.PostHandler) {
	apiRouter := r.Group("/api")

	// Set public routes
	apiRouter.GET("/feed", postHandler.GetFeed)

	postRouter := apiRouter.Group("/posts")

	// Require authentication for the remaining post routes
	postRouter.Use(middleware.RequireAuthMiddleware())
	apiRouter.Use(middleware.SetClaimsMiddleware())

	// Set authenticated routes
	postRouter.POST("", middleware.ValidateAndSanitizeStruct(&schema.CreatePostRequest{}), postHandler.CreatePost)
}
