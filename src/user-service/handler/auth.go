package handler

import (
	"context"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authenticationService struct {
	pb.UnimplementedAuthenticationServiceServer
}

func NewAuthenticationServer() pb.AuthenticationServiceServer {
	return &authenticationService{}
}

func (as authenticationService) Register(context.Context, *pb.RegisterRequest) (*pb.Profile, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Register not implemented")
}
func (as authenticationService) Activate(context.Context, *pb.ActivateRequest) (*pb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Activate not implemented")
}
func (as authenticationService) Login(context.Context, *pb.LoginRequest) (*pb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Login not implemented")
}
