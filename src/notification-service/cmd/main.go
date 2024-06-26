package main

import (
	"context"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/notification-service/handler"
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
	name    = "notification-service"
	version = "0.1.0"
)

func main() {
	// Initialize logger
	logger, cleanup := sharedLogging.InitializeLogger(name)
	defer cleanup()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
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
	userProfileClient := userv1.NewUserServiceClient(cfg.GRPCClients.UserService)
	userSubscriptionClient := userv1.NewSubscriptionServiceClient(cfg.GRPCClients.UserService)

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register notification service
	notificationv1.RegisterNotificationServiceServer(grpcServer, handler.NewNotificationServiceServer(logger, database, userProfileClient, userSubscriptionClient))
	notificationv1.RegisterPushServiceServer(grpcServer, handler.NewPushSubscriptionServiceServer(logger, database, userProfileClient, userSubscriptionClient))

	// Create listener
	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Fatalf("Failed to listen: %v", err)
	}

	// Start server
	logger.Infof("Starting %s v%s on port %s", name, version, cfg.Port)
	if err = grpcServer.Serve(lis); err != nil {
		logger.Fatalf("Failed to serve: %v", err)
	}
}
