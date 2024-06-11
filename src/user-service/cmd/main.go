package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/timeout"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wwi21seb-projekt/alpha-services/src/user-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	name    = "user-service"
	version = "0.1.0"
)

func InitTelemetry(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Create a resource
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(name),
		semconv.ServiceVersionKey.String(version),
	)

	// Create the Jaeger exporter
	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(traceExp),
		tracesdk.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Create prometheus exporter
	metricExp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	// Create the metric provider
	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		metric.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, mp.Shutdown)
	otel.SetMeterProvider(mp)

	return shutdown, nil
}

func main() {
	// Initialize logger
	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logTraceID := func(ctx context.Context) logging.Fields {
		if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
			return logging.Fields{"traceID", span.TraceID().String()}
		}
		return nil
	}
	opts := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
		logging.WithDurationField(logging.DefaultDurationToFields),
		logging.WithLevels(logging.DefaultServerCodeToLevel),
		logging.WithFieldsFromContext(logTraceID),
	}

	// Construct the DSN (Data Source Name) for the database connection
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable search_path=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword, cfg.SchemaName)

	// Initialize the database
	database, err := db.NewDB(dsn)
	if err != nil {
		logger.Fatal("Failed to connect to the database", zap.Error(err))
	}
	defer database.Close()

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	// Initialize prometheus metrics for gRPC server
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	reg := prometheus.NewRegistry()
	reg.MustRegister(srvMetrics)

	// Initialize prometheus metrics for gRPC client
	clMetrics := grpcprom.NewClientMetrics(
		grpcprom.WithClientHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	reg.MustRegister(clMetrics)

	// Setup metric for panic recoveries.
	panicsTotal := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
	grpcPanicRecoveryHandler := func(p any) (err error) {
		panicsTotal.Inc()
		logger.Info("recovered from panic", zap.Any("panic", p))
		return status.Errorf(codes.Internal, "%s", p)
	}

	// Create server
	otelgrpc.NewServerHandler()

	var serverOpts []grpc.ServerOption
	zapLogger := logger.With(zap.String("service", name)).Desugar()
	serverOpts = append(serverOpts,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(ExemplarFromContext)),
			logging.UnaryServerInterceptor(InterceptorLogger(zapLogger), opts...),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
	)
	grpcServer := grpc.NewServer(serverOpts...)

	// Create client connections
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			timeout.UnaryClientInterceptor(1*time.Second),
			clMetrics.UnaryClientInterceptor(grpcprom.WithExemplarFromContext(ExemplarFromContext)),
			logging.UnaryClientInterceptor(InterceptorLogger(zapLogger), opts...),
		),
	)
	mailConn, err := grpc.NewClient(cfg.MailServiceURL, dialOpts...)
	if err != nil {
		logger.Fatal("Failed to connect to mail service", zap.Error(err))
	}

	// Create client stubs
	mailClient := pbMail.NewMailServiceClient(mailConn)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Register user services
	pbUser.RegisterUserServiceServer(grpcServer, handler.NewUserServer(database))
	pbUser.RegisterSubscriptionServiceServer(grpcServer, handler.NewSubscriptionServer(database))
	pbUser.RegisterAuthenticationServiceServer(grpcServer, handler.NewAuthenticationServer(database, mailClient))

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

func ExemplarFromContext(ctx context.Context) prometheus.Labels {
	if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
		return prometheus.Labels{"traceID": span.TraceID().String()}
	}
	return nil
}

// InterceptorLogger adapts zap logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
