package handler

import (
	"context"

	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type subscriptionService struct {
	pb.UnimplementedSubscriptionServiceServer
}

func NewSubscriptionServer() pb.SubscriptionServiceServer {
	return &subscriptionService{}
}

func (ss subscriptionService) ListSubscriptions(context.Context, *pb.ListSubscriptionsRequest) (*pb.ListSubscriptionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSubscriptions not implemented")
}
func (ss subscriptionService) CreateSubscription(context.Context, *pb.CreateSubscriptionRequest) (*pb.Subscription, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateSubscription not implemented")
}
func (ss subscriptionService) DeleteSubscription(context.Context, *pb.DeleteSubscriptionRequest) (*pbCommon.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteSubscription not implemented")
}
