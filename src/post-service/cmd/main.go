package main

import (
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/post-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	name    = "post-service"
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
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize the database
	database, err := db.NewDB(cfg.DatabaseConfig)
	if err != nil {
		logger.Fatalf("Failed to connect to the database: %v", err)
	}
	defer database.Close()

	// Create listener
	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Fatalf("Failed to listen: %v", err)
	}

	// Create server
	var serverOpts []grpc.ServerOption
	grpcServer := grpc.NewServer(serverOpts...)

	// Create user client
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	userClient, err := grpc.NewClient(cfg.ServiceEndpoints.UserServiceURL, dialOpts...)
	if err != nil {
		logger.Fatalf("Failed to connect to user service: %v", err)
	}
	defer userClient.Close()

	// Create client stubs
	userProfileClient := pbUser.NewUserServiceClient(userClient)
	userSubscriptionClient := pbUser.NewSubscriptionServiceClient(userClient)

	// Register post service
	pbPost.RegisterPostServiceServer(grpcServer, handler.NewPostServiceServer(database, userProfileClient, userSubscriptionClient))

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Start server
	logger.Infof("Starting %s v%s on port %s", name, version, cfg.Port)
	if err = grpcServer.Serve(lis); err != nil {
		logger.Fatalf("Failed to serve: %v", err)
	}
}
