package main

import (
	grpcc "github.com/go-micro/plugins/v4/client/grpc"
	grpcs "github.com/go-micro/plugins/v4/server/grpc"
	"github.com/wwi21seb-projekt/alpha-services/post-service/handler"
	"go-micro.dev/v4"
	"go-micro.dev/v4/logger"

	pb "github.com/wwi21seb-projekt/alpha-services/post-service/proto"
)

var (
	name    = "post-service"
	version = "0.1.0"
)

func main() {
	// Create a new service
	srv := micro.NewService(
		micro.Server(grpcs.NewServer()),
		micro.Client(grpcc.NewClient()),
	)

	// Configure the service
	opts := []micro.Option{
		micro.Name(name),
		micro.Version(version),
	}

	// Initialize flags
	srv.Init(opts...)

	// Register handler
	/*
		if err := post.RegisterPostServiceHandler(srv.Server(), new(PostService)); err != nil {
			logger.Fatal(err)
		}
	*/
	if err := pb.RegisterHealthHandler(srv.Server(), new(handler.Health)); err != nil {
		logger.Fatal(err)
	}

	// Run the service
	if err := srv.Run(); err != nil {
		logger.Fatal(err)
	}
}
