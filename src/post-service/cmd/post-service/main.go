package main

import (
	"flag"
	"github.com/wwi21seb-projekt/alpha-services/src/post-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
)

var (
	name     = "post-service"
	version  = "0.1.0"
	port     = *flag.String("port", os.Getenv("port"), "Port to listen on")
	userAddr = *flag.String("userAddr", os.Getenv("userAddr"), "Address of the user service")
)

func main() {
	// Parse flags
	flag.Parse()

	// Initialize empty database
	database, _ := db.NewDB("localhost")
	defer database.Close()

	// Create listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create server
	var serverOpts []grpc.ServerOption
	grpcServer := grpc.NewServer(serverOpts...)

	// Create user client
	/*
		var dialOpts []grpc.DialOption
		conn, err := grpc.NewClient(userAddr, dialOpts...)
		if err != nil {
			log.Fatalf("Failed to connect to user service: %v", err)
		}
		defer conn.Close()

		userClient := pb.NewUserServiceClient(conn)
	*/

	// Register post service
	pbPost.RegisterPostServiceServer(grpcServer, handler.NewPostServiceServer(database, nil))

	// Register health service
	pbHealth.RegisterHealthServer(grpcServer, handler.NewHealthServer())

	// Start server
	log.Printf("Starting %s v%s on port %s", name, version, port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
