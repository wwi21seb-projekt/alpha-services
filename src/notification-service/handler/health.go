package handler

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/wwi21seb-projekt/alpha-shared/proto/health"
)

type healthService struct {
	pb.UnimplementedHealthServer
}

func NewHealthServer() pb.HealthServer {
	return &healthService{}
}

func (h *healthService) Check(context.Context, *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status: pb.HealthCheckResponse_SERVING,
	}, nil
}

func (h *healthService) Watch(*pb.HealthCheckRequest, pb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}
