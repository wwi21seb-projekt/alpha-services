package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wwi21seb-projekt/alpha-services/src/user-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	sharedLogging "github.com/wwi21seb-projekt/alpha-shared/logging"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"github.com/wwi21seb-projekt/alpha-shared/tracing"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	name    = "user-service"
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
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize the database
	database, err := db.NewDB(cfg.DatabaseConfig)
	if err != nil {
		logger.Fatal("Failed to connect to the database", zap.Error(err))
	}
	defer database.Close()

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	// Initialize prometheus metrics for gRPC server and client
	reg := prometheus.NewRegistry()
	srvMetrics := tracing.GetServerMetrics()
	clMetrics := tracing.GetClientMetrics()
	reg.MustRegister(srvMetrics)
	reg.MustRegister(clMetrics)

	// Setup metric for panic recoveries.
	var panicCounter uint64 = 0
	promauto.With(reg).NewCounterFunc(
		prometheus.CounterOpts{
			Name: "grpc_req_panics_recovered_total",
			Help: "Total number of gRPC requests recovered from internal panic.",
		},
		func() float64 {
			return float64(atomic.LoadUint64(&panicCounter))
		},
	)

	// Init telemetry and create server
	shutdown, err := tracing.InitTelemetry(context.Background(), name, version)
	if err != nil {
		logger.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			logger.Fatal("Failed to shutdown telemetry", zap.Error(err))
		}
	}()
	otelgrpc.NewServerHandler()

	zapLogger := logger.With(zap.String("service", name)).Desugar()
	grpcServer := grpc.NewServer(tracing.NewServerOptions(srvMetrics, zapLogger, &panicCounter)...)

	// Create client connections
	mailConn, err := grpc.NewClient(cfg.ServiceEndpoints.MailServiceURL, tracing.NewClientOptions(clMetrics, zapLogger)...)
	if err != nil {
		logger.Fatal("Failed to connect to mail service", zap.Error(err))
	}

	// Create client stubs
	mailClient := pbMail.NewMailServiceClient(mailConn)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Register user services
	pbUser.RegisterUserServiceServer(grpcServer, handler.NewUserServer(logger, database))
	pbUser.RegisterSubscriptionServiceServer(grpcServer, handler.NewSubscriptionServer(logger, database))
	pbUser.RegisterAuthenticationServiceServer(grpcServer, handler.NewAuthenticationServer(logger, database, mailClient))

	// Initialize server metrics
	srvMetrics.InitializeMetrics(grpcServer)

	// Start metrics server
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			ErrorHandling:     promhttp.ContinueOnError,
			EnableOpenMetrics: true,
		}))
		logger.Info("Metrics server started on port 2112")
		http.ListenAndServe(":2112", nil)
	}()

	// Start server
	logger.Infof("Starting %s v%s on port %s", name, version, cfg.Port)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("Failed to serve", zap.Error(err))
	}
}
