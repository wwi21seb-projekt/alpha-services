package handler

import (
	"context"

	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userService struct {
	pb.UnimplementedUserServiceServer
}

func NewUserServer() pb.UserServiceServer {
	return &userService{}
}

func (ps userService) GetUser(ctx context.Context, request *pb.GetUserRequest) (*pb.User, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetProfile not implemented")
}

func (ps userService) UpdateUser(ctx context.Context, request *pb.UpdateUserRequest) (*pb.User, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateProfile not implemented")
}

func (ps userService) SearchUsers(ctx context.Context, request *pb.SearchUsersRequest) (*pb.ListUsersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SearchProfiles not implemented")
}

func (ps userService) ListUsers(ctx context.Context, request *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProfiles not implemented")
}
