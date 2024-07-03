package main

import (
	"context"
	"fmt"
	"net"

	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"

	"github.com/wwi21seb-projekt/alpha-services/src/user-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedGRPC "github.com/wwi21seb-projekt/alpha-shared/grpc"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	name    = "user-service"
	version = "0.1.0"
)

func main() {
	// Initialize logger
	logger, cleanup := sharedLogging.InitializeLogger(name)
	defer cleanup()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize the database
	ctx := context.Background()
	database, err := db.NewDB(ctx, cfg.DatabaseConfig, logger)
	if err != nil {
		logger.Fatal("Failed to connect to the database", zap.Error(err))
	}
	defer database.Close()

	// Initialize tracing
	tracingShutdown, err := tracing.InitializeTracing(ctx, name, version)
	if err != nil {
		logger.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer tracingShutdown()

	// Intialize metrics
	metricShutdown, err := metrics.InitializeMetrics(ctx, name, version)
	if err != nil {
		logger.Fatal("Failed to initialize metrics", zap.Error(err))
	}
	defer metricShutdown()

	// Initialize gRPC client connections
	if err = cfg.InitializeClients(logger.Desugar()); err != nil {
		logger.Fatal("Failed to initialize gRPC clients", zap.Error(err))
	}

	// Create client stubs
	mailClient := notificationv1.NewMailServiceClient(cfg.GRPCClients.NotificationService)
	notificationClient := notificationv1.NewNotificationServiceClient(cfg.GRPCClients.NotificationService)
	imageClient := imagev1.NewImageServiceClient(cfg.GRPCClients.ImageService)
	postClient := postv1.NewPostServiceClient(cfg.GRPCClients.PostService)

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register user services
	userv1.RegisterUserServiceServer(grpcServer, handler.NewUserServer(logger, database, postClient, imageClient))
	userv1.RegisterSubscriptionServiceServer(grpcServer, handler.NewSubscriptionServer(logger, database, notificationClient))
	userv1.RegisterAuthenticationServiceServer(grpcServer, handler.NewAuthenticationServer(logger, database, mailClient, imageClient))

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	// Start server
	logger.Infof("Starting %s v%s on port %s", name, version, cfg.Port)
	if err = grpcServer.Serve(lis); err != nil {
		logger.Fatal("Failed to serve", zap.Error(err))
	}
}
