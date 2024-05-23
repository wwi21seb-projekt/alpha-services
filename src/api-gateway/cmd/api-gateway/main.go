package main

import (
	"context"
	"errors"
	"github.com/gin-contrib/graceful"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"go-micro.dev/v4"
	"go-micro.dev/v4/logger"
	"os/signal"
	"syscall"

	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
)

func main() {
	// Create a go-micro service
	srv := micro.NewService(
		micro.Name("api-gateway"),
		micro.Version("latest"),
	)
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
	apiRouter.Use(middleware.SetClaimsMiddleware())

	postRouter := apiRouter.Group("/posts")
	postRouter.Use(middleware.RequireAuthMiddleware())
	postRouter.POST("", middleware.ValidateAndSanitizeStruct(&schema.CreatePostRequest{}), postHandler.CreatePost)
}
