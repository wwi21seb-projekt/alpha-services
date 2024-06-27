package handler

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"os"
)

type pushSubscriptionService struct {
	logger             *zap.SugaredLogger
	db                 *db.DB
	profileClient      userv1.UserServiceClient
	subscriptionClient userv1.SubscriptionServiceClient
	notificationv1.UnimplementedPushServiceServer
}

func NewPushSubscriptionServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient userv1.UserServiceClient, subscriptionClient userv1.SubscriptionServiceClient) notificationv1.PushServiceServer {
	return &pushSubscriptionService{
		logger:             logger,
		db:                 db,
		profileClient:      profileClient,
		subscriptionClient: subscriptionClient,
	}
}

func (p *pushSubscriptionService) GetPublicKey(context.Context, *notificationv1.GetPublicKeyRequest) (*notificationv1.GetPublicKeyResponse, error) {
	vapidPulicKey := os.Getenv("VAPID_PUBLIC_KEY")
	return &notificationv1.GetPublicKeyResponse{
		PublicKey: vapidPulicKey,
	}, nil
}

func (p *pushSubscriptionService) CreatePushSubscription(ctx context.Context, request *notificationv1.CreatePushSubscriptionRequest) (*notificationv1.CreatePushSubscriptionResponse, error) {
	conn, err := p.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := p.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer p.db.RollbackTx(ctx, tx)

	subscriptionId := uuid.New()
	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Check if the authenticated user has any subscriptions with the same type and a future expiration time
	p.logger.Info("Checking for existing subscription with the same username, type, and future expiration time...")

	// Type needs to be converted to lowercase because enum value is uppercase but postgres expects lowercase

	var typeLower string
	if request.Type == notificationv1.PushSubscriptionType_PUSH_SUBSCRIPTION_TYPE_WEB {
		typeLower = "web"
	} else {
		typeLower = "expo"
	}

	var expirationTime pgtype.Timestamptz
	if request.ExpirationTime != "" {
		err = expirationTime.Scan(request.ExpirationTime)
		if err != nil {
			p.logger.Errorf("Error in expirationTime.Scan: %v", err)
			expirationTime = pgtype.Timestamptz{Valid: false}
		}
	}

	p.logger.Info("Inserting subscription into database...")
	query, args, _ := psql.Insert("push_subscriptions").
		Columns("subscription_id", "username", "type", "endpoint", "expiration_time", "p256dh", "auth").
		Values(subscriptionId, authenticatedUsername, typeLower, request.Endpoint, expirationTime, request.P256Dh, request.Auth).
		ToSql()
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	if err := p.db.CommitTx(ctx, tx); err != nil {
		p.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &notificationv1.CreatePushSubscriptionResponse{
		SubscriptionId: subscriptionId.String(),
	}, nil
}
