package main

import (
	"context"
	"errors"
	"github.com/gin-contrib/graceful"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/micro_utils"
	"go-micro.dev/v4"
	"go-micro.dev/v4/logger"
	"os/signal"
	"syscall"

	grpcc "github.com/go-micro/plugins/v4/client/grpc"
	_ "github.com/go-micro/plugins/v4/registry/kubernetes"
	grpcs "github.com/go-micro/plugins/v4/server/grpc"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
)

var (
	name    = "api-gateway"
	version = "1.0.0"
)

func main() {
	// Load configuration
	if err := micro_utils.Load(); err != nil {
		logger.Fatal(err)
	}

	// Create a go-micro service
	srv := micro.NewService(
		micro.Server(grpcs.NewServer()),
		micro.Client(grpcc.NewClient()),
	)
	opts := []micro.Option{
		micro.Name(name),
		micro.Version(version),
		micro.Address(micro_utils.Address()),
	}

	srv.Init()

	// Create client stub for post-service
	postService := pbPost.NewPostService("post-service", srv.Client())
	// Create an instance of PostHandler
	postHandler := handler.NewPostHandler(postService)

	// Expose HTTP endpoint with go-micro server
	r, err := graceful.Default()
	if err != nil {
		logger.Fatal(err)
	}

	setupRoutes(r, postHandler)

	// Create a context that listens for termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a channel to signal termination
	quit := make(chan struct{})

	// Run the service asynchronously
	go func() {
		if err = srv.Run(); err != nil {
			logger.Info(err)
		}
		close(quit)
	}()

	// Run gin router asynchronously
	go func() {
		if err = r.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Info(err)
		}
		close(quit)
	}()

	// Wait for either service to terminate
	<-quit

	// Stop the service
	if err := srv.Server().Stop(); err != nil {
		logger.Info(err)
	}

	// Stop the gin router gracefully
	if err := r.Stop(); err != nil {
		logger.Info(err)
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
