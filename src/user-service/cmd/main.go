package main

import (
	"flag"
	"fmt"
	"github.com/wwi21seb-projekt/alpha-services/src/user-service/handler"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
)

var (
	name    = "user-service"
	version = "0.1.0"
	port    = *flag.String("port", os.Getenv("PORT"), "Port to listen on")
)

func main() {
	// Parse flags
	flag.Parse()

	// Initialize database connection
	// database, _ := db.NewDB("localhost")
	// defer database.Close()

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create server
	var serverOpts []grpc.ServerOption
	grpcServer := grpc.NewServer(serverOpts...)

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Register user services
	pbUser.RegisterProfileServiceServer(grpcServer, handler.NewProfileServer())
	pbUser.RegisterSubscriptionServiceServer(grpcServer, handler.NewSubscriptionServer())
	pbUser.RegisterAuthenticationServiceServer(grpcServer, handler.NewAuthenticationServer())

	// Start server
	log.Printf("Starting %s v%s on port %s", name, version, port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
