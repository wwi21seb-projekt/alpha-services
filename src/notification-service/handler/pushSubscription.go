package handler

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

type pushSubscriptionService struct {
	logger             *zap.SugaredLogger
	db                 *db.DB
	profileClient      pbUser.UserServiceClient
	subscriptionClient pbUser.SubscriptionServiceClient
	pb.UnimplementedPushServiceServer
}

func NewPushSubscriptionServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient, subscriptionClient pbUser.SubscriptionServiceClient) pb.PushServiceServer {
	return &pushSubscriptionService{
		logger:             logger,
		db:                 db,
		profileClient:      profileClient,
		subscriptionClient: subscriptionClient,
	}
}

func (p *pushSubscriptionService) GetPublicKey(context.Context, *pbCommon.Empty) (*pb.PublicKeyResponse, error) {
	vapidPulicKey := os.Getenv("VAPID_PUBLIC_KEY")
	return &pb.PublicKeyResponse{
		PublicKey: vapidPulicKey,
	}, nil
}

func (p *pushSubscriptionService) CreatePushSubscription(ctx context.Context, request *pb.CreatePushSubscriptionRequest) (*pb.CreatePushSubscriptionResponse, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		p.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer p.db.Rollback(ctx, tx)

	subscriptionId := uuid.New()
	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	p.logger.Info("Checking for existing subscription with the same username, type, and future expiration time...")

	// Check if the authenticated user has any subscriptions with the same type and a future expiration time
	p.logger.Info("Checking for existing subscription with the same username, type, and future expiration time...")

	subscriptionsQuery, subscriptionsArgs, _ := psql.Select().
		Columns("s.subscription_id", "s.type", "s.token", "s.endpoint", "s.expiration_time", "s.p256dh", "s.auth").
		From("push_subscriptions s").
		Where("s.username = ?", authenticatedUsername).
		Where("s.type = ?", "web").
		Where("s.expiration_time > ?", time.Now()).
		ToSql()

	subscriptionRows, err := tx.Query(ctx, subscriptionsQuery, subscriptionsArgs...)
	if err != nil {
		p.logger.Errorf("Error executing query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error executing query: %v", err)
	}
	defer subscriptionRows.Close()

	if subscriptionRows.Next() {
		p.logger.Info("A subscription with the same username and type already exists with a future expiration time.")
		return nil, errors.New("subscription already exists")
	}

	p.logger.Info("Inserting subscription into database...")
	// Type needs to be converted to lowercase because enum value is uppercase but postgres expects lowercase
	typeLower := strings.ToLower(request.Type.String())
	query, args, _ := psql.Insert("push_subscriptions").
		Columns("subscription_id", "username", "type", "endpoint", "expiration_time", "p256dh", "auth").
		Values(subscriptionId, authenticatedUsername, typeLower, request.Endpoint, request.ExpirationTime, request.P256Dh, request.Auth).
		ToSql()
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	if err := p.db.Commit(ctx, tx); err != nil {
		p.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pb.CreatePushSubscriptionResponse{
		SubscriptionId: subscriptionId.String(),
	}, nil
}
