package main

import (
	"context"
	"fmt"
	"net"

	"github.com/wwi21seb-projekt/alpha-services/src/mail-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	sharedGRPC "github.com/wwi21seb-projekt/alpha-shared/grpc"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	"github.com/wwi21seb-projekt/alpha-shared/metrics"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	name    = "mail-service"
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

	// Initialize tracing
	ctx := context.Background()
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

	// Create the gRPC Server
	grpcServer := grpc.NewServer(sharedGRPC.NewServerOptions(logger.Desugar())...)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())
	pb.RegisterMailServiceServer(grpcServer, handler.NewMailService(logger))

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	// Start server
	logger.Infof("Starting %s v%s on port %s", name, version, cfg.Port)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatalf("Failed to serve: %v", err)
	}
}
