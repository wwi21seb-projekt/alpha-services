package main

import (
	grpcc "github.com/go-micro/plugins/v4/client/grpc"
	grpcs "github.com/go-micro/plugins/v4/server/grpc"
	"github.com/wwi21seb-projekt/alpha-services/src/post-service/handler"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"go-micro.dev/v4"
	"go-micro.dev/v4/auth"
	"go-micro.dev/v4/logger"

	pbHealth "github.com/wwi21seb-projekt/alpha-shared/proto/health"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
)

var (
	name    = "post-service"
	version = "0.1.0"
)

func main() {
	// Create a new auth provider
	authProvider := auth.NewAuth(
		auth.Namespace("com.example.srv.post"),
		auth.PublicKey("key"),  // TODO: Replace with actual public key
		auth.PrivateKey("key"), // TODO: Replace with actual private key
		auth.Addrs("localhost:8080"),
	)

	// Create a new service
	srv := micro.NewService(
		micro.Server(grpcs.NewServer()),
		micro.Client(grpcc.NewClient()),
		micro.Auth(authProvider),
	)

	// Configure the service
	opts := []micro.Option{
		micro.Name(name),
		micro.Version(version),
	}

	// Initialize flags
	srv.Init(opts...)

	// Initialize empty database
	db, _ := db.NewDB("localhost")

	// Initialize userService
	userService := pb.NewUserService("com.example.srv.user", srv.Client())

	// Register handler
	if err := pb.RegisterPostServiceHandler(srv.Server(), handler.NewPostService(db, userService)); err != nil {
		logger.Fatal(err)
	}
	if err := pbHealth.RegisterHealthHandler(srv.Server(), new(handler.Health)); err != nil {
		logger.Fatal(err)
	}

	// Run the service
	if err := srv.Run(); err != nil {
		logger.Fatal(err)
	}
}
