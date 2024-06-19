package main

import (
	"context"
	"fmt"
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/user-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedGRPC "github.com/wwi21seb-projekt/alpha-shared/grpc"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbImage "github.com/wwi21seb-projekt/alpha-shared/proto/image"
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pbNotification "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
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
	mailClient := pbMail.NewMailServiceClient(cfg.GRPCClients.MailService)
	notificationClient := pbNotification.NewNotificationServiceClient(cfg.GRPCClients.NotificationService)
	imageClient := pbImage.NewImageServiceClient(cfg.GRPCClients.ImageService)

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Register user services
	pbUser.RegisterUserServiceServer(grpcServer, handler.NewUserServer(logger, database))
	pbUser.RegisterSubscriptionServiceServer(grpcServer, handler.NewSubscriptionServer(logger, database, notificationClient))
	pbUser.RegisterAuthenticationServiceServer(grpcServer, handler.NewAuthenticationServer(logger, database, mailClient, imageClient))

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
