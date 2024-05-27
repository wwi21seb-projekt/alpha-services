package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/mail-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	"google.golang.org/grpc"
)

var (
	name    = "mail-service"
	version = "0.1.0"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initizalize logger
	logger := logrus.New()

	opts := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
		logging.WithDurationField(logging.DefaultDurationToFields),
		logging.WithLevels(logging.DefaultServerCodeToLevel),
	}

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create server
	var serverOpts []grpc.ServerOption
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(
		logging.UnaryServerInterceptor(InterceptorLogger(logger), opts...),
	))
	grpcServer := grpc.NewServer(serverOpts...)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())
	pb.RegisterMailServiceServer(grpcServer, handler.NewMailService())

	// Start server
	log.Printf("Starting %s v%s on port %s", name, version, cfg.Port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func InterceptorLogger(l logrus.FieldLogger) logging.Logger {
	return logging.LoggerFunc(func(_ context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make(map[string]any, len(fields)/2)
		i := logging.Fields(fields).Iterator()
		for i.Next() {
			k, v := i.At()
			f[k] = v
		}
		l := l.WithFields(f)

		switch lvl {
		case logging.LevelDebug:
			l.Debug(msg)
		case logging.LevelInfo:
			l.Info(msg)
		case logging.LevelWarn:
			l.Warn(msg)
		case logging.LevelError:
			l.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
