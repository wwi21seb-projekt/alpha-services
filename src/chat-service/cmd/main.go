package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/chat-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	name    = "chat-service"
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

	// Construct the DSN (Data Source Name) for the database connection
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable search_path=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword, cfg.SchemaName)

	// Initialize the database
	database, err := db.NewDB(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
	defer database.Close()

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

	// Create client connections
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	userConn, err := grpc.NewClient(cfg.UserServiceURL, dialOpts...)
	if err != nil {
		log.Fatalf("Failed to connect to mail service: %v", err)
	}

	// Create client stubs
	userClient := pbUser.NewUserServiceClient(userConn)

	// Register services
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())
	pb.RegisterChatServiceServer(grpcServer, handler.NewChatService(database, userClient))

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