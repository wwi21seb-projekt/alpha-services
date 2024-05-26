package handler

import (
	"context"
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

func (ss subscriptionService) GetSubscriptions(context.Context, *pb.GetSubscriptionsRequest) (*pb.Subscriptions, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSubscriptions not implemented")
}
func (ss subscriptionService) Subscribe(context.Context, *pb.SubscribeRequest) (*pb.Subscription, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}
func (ss subscriptionService) Unsubscribe(context.Context, *pb.UnsubscribeRequest) (*pb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Unsubscribe not implemented")
}
