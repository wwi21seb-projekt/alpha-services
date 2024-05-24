package api_gateway_vanilla

import (
	"context"
	"errors"
	"flag"
	"github.com/gin-contrib/graceful"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/handler"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	"go-micro.dev/v4/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	postGrpcURL = *flag.String("post_grpc_url", os.Getenv("POST_GRPC_URL"), "URL of the gRPC server for posts")
)

func main() {
	// Parse the command-line flags
	flag.Parse()

	// Set up a connection to the gRPC server
	postConn, err := grpc.Dial(postGrpcURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer postConn.Close()

	// Create client stubs
	postClientStub := pbPost.NewPostServiceClient(postConn)

	// Create handler instances
	postHandler := handler.NewPostHandler(postClientStub)

	// Expose HTTP endpoint with go-micro server
	r, err := graceful.Default()
	if err != nil {
		logger.Fatal(err)
	}

	setupRoutes(r, postHandler)

	// Create a context that listens for termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err = r.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
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
