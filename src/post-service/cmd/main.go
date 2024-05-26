package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/post-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/config"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	"google.golang.org/grpc"
	"net"
)

var (
	name    = "post-service"
	version = "0.1.0"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
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
	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create server
	var serverOpts []grpc.ServerOption
	grpcServer := grpc.NewServer(serverOpts...)

	// Register post service
	pbPost.RegisterPostServiceServer(grpcServer, handler.NewPostServiceServer(database, nil))

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Start server
	log.Printf("Starting %s v%s on port %s", name, version, cfg.Port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
