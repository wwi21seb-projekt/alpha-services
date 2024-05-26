package handler

import (
	"context"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type profileService struct {
	pb.UnimplementedProfileServiceServer
}

func NewProfileServer() pb.ProfileServiceServer {
	return &profileService{}
}

func (ps profileService) GetProfile(context.Context, *pb.Empty) (*pb.Profile, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetProfile not implemented")
}
func (ps profileService) SearchProfiles(context.Context, *pb.SearchProfilesRequest) (*pb.PaginatedProfiles, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SearchProfiles not implemented")
}
func (ps profileService) GetFeed(context.Context, *pb.GetFeedRequest) (*pb.PaginatedPosts, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetFeed not implemented")
}
func (ps profileService) ChangeTrivialInfo(context.Context, *pb.ChangeTrivialInfoRequest) (*pb.TrivialInfo, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ChangeTrivialInfo not implemented")
}
func (ps profileService) ChangePassword(context.Context, *pb.ChangePasswordRequest) (*pb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ChangePassword not implemented")
}
