package main

import (
	"context"
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/notification-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedGRPC "github.com/wwi21seb-projekt/alpha-shared/grpc"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbNotification "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
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
	database, err := db.NewDB(ctx, cfg.DatabaseConfig)
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
	userProfileClient := pbUser.NewUserServiceClient(cfg.GRPCClients.UserService)
	userSubscriptionClient := pbUser.NewSubscriptionServiceClient(cfg.GRPCClients.UserService)

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register notification service
	pbNotification.RegisterNotificationServiceServer(grpcServer, handler.NewNotificationServiceServer(logger, database, userProfileClient, userSubscriptionClient))
	pbNotification.RegisterPushServiceServer(grpcServer, handler.NewPushSubscriptionServiceServer(logger, database, userProfileClient, userSubscriptionClient))

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

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
