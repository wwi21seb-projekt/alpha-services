package main

import (
	"context"
	"fmt"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/post-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedGRPC "github.com/wwi21seb-projekt/alpha-shared/grpc"
	"github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	name    = "post-service"
	version = "0.1.0"
)

func main() {
	ctx := context.Background()

	logger, cleanup := logging.InitializeLogger(name)
	defer cleanup()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	database, err := db.NewDB(ctx, cfg.DatabaseConfig, logger)
	if err != nil {
		logger.Fatal("Failed to connect to the database", zap.Error(err))
	}
	defer database.Close()

	tracingShutdown, err := tracing.InitializeTracing(ctx, name, version)
	if err != nil {
		logger.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer tracingShutdown()

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
	userProfileClient := userv1.NewUserServiceClient(cfg.GRPCClients.UserService)
	userSubscriptionClient := userv1.NewSubscriptionServiceClient(cfg.GRPCClients.UserService)
	imageClient := imagev1.NewImageServiceClient(cfg.GRPCClients.ImageService)
	notificationClient := notificationv1.NewNotificationServiceClient(cfg.GRPCClients.NotificationService)

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register post service
	postv1.RegisterInteractionServiceServer(grpcServer, handler.NewInteractionService(logger, database, userProfileClient))
	postv1.RegisterPostServiceServer(grpcServer, handler.NewPostServiceServer(logger, database, userProfileClient, userSubscriptionClient, imageClient, notificationClient))

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
