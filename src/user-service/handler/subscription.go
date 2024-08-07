package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type subscriptionService struct {
	logger             *zap.SugaredLogger
	tracer             trace.Tracer
	db                 *db.DB
	notificationClient notificationv1.NotificationServiceClient
	userv1.UnimplementedSubscriptionServiceServer
}

func NewSubscriptionServer(logger *zap.SugaredLogger, database *db.DB, notificiationClient notificationv1.NotificationServiceClient) userv1.SubscriptionServiceServer {
	return &subscriptionService{
		logger:             logger,
		tracer:             otel.GetTracerProvider().Tracer("subscription-service"),
		db:                 database,
		notificationClient: notificiationClient,
	}
}

func (ss subscriptionService) ListSubscriptions(ctx context.Context, request *userv1.ListSubscriptionsRequest) (*userv1.ListSubscriptionsResponse, error) {
	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Get the followers by default
	userTypes := []string{"subscriber_name", "subscribee_name"}
	// If the subscription type is following, fetch the users the user is following
	if request.GetSubscriptionType() == userv1.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING {
		userTypes = []string{"subscribee_name", "subscriber_name"}
	}

	offset, err := strconv.ParseInt(request.GetPagination().GetPageToken(), 10, 64)
	if err != nil {
		offset = 0
	}

	// For every subscription we also want to know if the authenticated user is subscribed to the subscribee and vice
	// versa. Table S1 is the current subscriber or subscribee in the loop of the user we are querying, S2 represents
	// the subscription of the current user in the loop to the authenticated user and S3 the other way around.
	selectCtx, selectSpan := ss.tracer.Start(ctx, "GetSubscriptionsData")
	dataQuery, dataArgs, _ := psql.Select().
		Columns("s2.subscription_id AS follower_subscription_id").
		Columns("s3.subscription_id AS followed_subscription_id").
		Columns("u.username", "u.nickname", "u.picture_url", "u.picture_width", "u.picture_height").
		From("users u").
		Join("subscriptions s1 ON s1."+userTypes[0]+" = u.username").
		LeftJoin("subscriptions s2 ON s2.subscriber_name = ? AND s2.subscribee_name = u.username", authenticatedUsername).
		LeftJoin("subscriptions s3 ON s3.subscriber_name = u.username AND s3.subscribee_name = ?", authenticatedUsername).
		Where("s1."+userTypes[1]+" = ?", request.GetUsername()).
		OrderBy("s1.created_at DESC").
		Limit(uint64(request.GetPagination().GetPageSize())).
		Offset(uint64(offset)).
		ToSql()

	// Count the total number of subscriptions and also validate that the user exists
	countQuery, countArgs, _ := psql.Select("COUNT(s.subscription_id)", "COUNT(u.username)").
		From("users u").
		// We need to use a left join here in case there's no subscriptions to list
		LeftJoin("subscriptions s ON s."+userTypes[1]+" = u.username").
		Where("u.username = ?", request.GetUsername()).
		ToSql()

	ss.logger.Info("Starting batch request for subscription data")
	batch := &pgx.Batch{}
	batch.Queue(countQuery, countArgs...)
	batch.Queue(dataQuery, dataArgs...)

	conn, err := ss.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	br := conn.SendBatch(selectCtx, batch)
	defer br.Close()

	// Get the first batch result which contains the total number of subscriptions and the user count
	var subscriptionCount, userCount int32
	if err = br.QueryRow().Scan(&subscriptionCount, &userCount); err != nil {
		ss.logger.Errorf("Error in br.QueryRow: %v", err)
		selectSpan.End()
		return nil, status.Errorf(codes.Internal, "Error in br.QueryRow: %v", err)
	}

	if userCount == 0 {
		ss.logger.Errorf("user %s does not exist", request.GetUsername())
		selectSpan.End()
		return nil, status.Errorf(codes.NotFound, "user does not exist")
	}

	// Get the second batch result which contains the subscription data
	rows, err := br.Query()
	if err != nil {
		ss.logger.Errorf("Error in br.Query: %v", err)
		selectSpan.End()
		return nil, status.Errorf(codes.Internal, "Error in br.Query: %v", err)
	}
	selectSpan.End()
	defer rows.Close()

	response := &userv1.ListSubscriptionsResponse{}
	_, scanRowsSpan := ss.tracer.Start(ctx, "ScanSubscriptionRows")
	defer scanRowsSpan.End() // we defer the span here, since we'll return after the loop anyway
	for rows.Next() {
		subscription := &userv1.Subscription{}
		var followedId, followerId, nickname, pictureUrl pgtype.Text
		var pictureWidth, pictureHeight pgtype.Int4

		if err = rows.Scan(&followedId, &followerId, &subscription.Username, &nickname, &pictureUrl, &pictureWidth, &pictureHeight); err != nil {
			ss.logger.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Errorf(codes.Internal, "Error in rows.Scan: %v", err)
		}

		// Set the fields if they are valid
		if followedId.Valid {
			subscription.FollowedSubscriptionId = followedId.String
		}
		if followerId.Valid {
			subscription.FollowerSubscriptionId = followerId.String
		}
		if nickname.Valid {
			subscription.Nickname = nickname.String
		}
		if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
			subscription.Picture = &imagev1.Picture{
				Url:    pictureUrl.String,
				Width:  pictureWidth.Int32,
				Height: pictureHeight.Int32,
			}
		}
		response.Subscriptions = append(response.Subscriptions, subscription)
	}

	response.Pagination = &commonv1.PaginationResponse{
		TotalSize:     subscriptionCount,
		NextPageToken: request.GetPagination().GetPageToken(),
	}

	return response, nil
}

func (ss subscriptionService) CreateSubscription(ctx context.Context, request *userv1.CreateSubscriptionRequest) (*userv1.CreateSubscriptionResponse, error) {
	conn, err := ss.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := ss.db.BeginTx(ctx, conn)
	if err != nil {
		ss.logger.Errorf("Error in ss.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer ss.db.RollbackTx(ctx, tx)

	// Fetch the username of the authenticated user
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Return early error if the user tries to subscribe to themselves
	if username == request.GetFollowedUsername() {
		ss.logger.Errorf("user %s tried to subscribe to themselves", username)
		return nil, status.Errorf(codes.InvalidArgument, "user %s tried to subscribe to themselves", username)
	}

	// Create the subscription within the transaction, we'll get a constraint violation if the
	// user does not exist or the subscription already exists
	insertCtx, insertSpan := ss.tracer.Start(ctx, "CreateSubscription")
	subscriptionId := uuid.New()
	createdAt := time.Now()
	query, args, _ := psql.Insert("subscriptions").
		Columns("subscription_id", "subscriber_name", "subscribee_name", "created_at").
		Values(subscriptionId, username, request.GetFollowedUsername(), createdAt).
		ToSql()

	ss.logger.Info("Creating subscription")
	if _, err = tx.Exec(insertCtx, query, args...); err != nil {
		insertSpan.End()
		// Check if the error is a constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
			// We need to differentiate between the two possible constraint violations
			// 1: Foreign key violation -> user does not exist
			// 2: Unique violation -> subscription already exists
			if pgErr.ConstraintName == "subscribee_fk" {
				ss.logger.Errorf("user %s does not exist", request.GetFollowedUsername())
				return nil, status.Error(codes.NotFound, "user does not exist")
			} else if pgErr.ConstraintName == "subscriptions_uq" {
				ss.logger.Errorf("subscription already exists")
				return nil, status.Errorf(codes.AlreadyExists, "subscription already exists")
			}
		}

		ss.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}
	insertSpan.End()

	if err = ss.db.CommitTx(ctx, tx); err != nil {
		ss.logger.Errorf("Error in ss.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in ss.db.Commit: %v", err)
	}

	// Send a notification to the user that they have been subscribed to
	sendNotificationRequest := notificationv1.SendNotificationRequest{
		NotificationType: 	"follow",
		Recipient:       	request.GetFollowedUsername(),
	}

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("sub", username))
	if _, err = ss.notificationClient.SendNotification(ctx, &sendNotificationRequest); err != nil {
		ss.logger.Error("Error in ss.notificationClient.SendNotification", zap.Error(err))
	}

	return &userv1.CreateSubscriptionResponse{
		SubscriptionId:   subscriptionId.String(),
		SubscriptionDate: createdAt.Format(time.RFC3339),
		FollowerUsername: username,
		FollowedUsername: request.GetFollowedUsername(),
	}, nil
}
func (ss subscriptionService) DeleteSubscription(ctx context.Context, request *userv1.DeleteSubscriptionRequest) (*userv1.DeleteSubscriptionResponse, error) {
	// Fetch the username of the authenticated user
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ss.logger.Infof("Deleting subscription %s", request.GetSubscriptionId())
	ss.logger.Infof("Authenticated user: %s", username)

	// Trigger the deletion of the subscription within the transaction, if the subscription does not exist
	// we'll get an appropriate error
	deleteCtx, deleteSpan := ss.tracer.Start(ctx, "DeleteSubscription")
	query, args, _ := psql.Delete("subscriptions").
		Where("subscription_id = ?", request.GetSubscriptionId()).
		// We return the subscriber_name to verify that the authenticated user is the subscriber
		Suffix("RETURNING subscriber_name").
		ToSql()

	conn, err := ss.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := ss.db.BeginTx(ctx, conn)
	if err != nil {
		ss.logger.Errorf("Error in ss.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer ss.db.RollbackTx(ctx, tx)

	ss.logger.Info("Deleting subscription...")
	var subscriberName string
	if err = tx.QueryRow(deleteCtx, query, args...).Scan(&subscriberName); err != nil {
		deleteSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			ss.logger.Errorf("subscription %s does not exist", request.GetSubscriptionId())
			return nil, status.Errorf(codes.NotFound, "subscription does not exist")
		}

		ss.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.QueryRow: %v", err)
	}
	deleteSpan.End()

	// Check if the authenticated user is the subscriber
	if subscriberName != username {
		ss.logger.Errorf("user %s tried to delete subscription %s", username, request.GetSubscriptionId())
		return nil, status.Errorf(codes.PermissionDenied, "the authenticated user is not the subscriber")
	}

	if err = ss.db.CommitTx(ctx, tx); err != nil {
		ss.logger.Errorf("Error in ss.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in ss.db.Commit: %v", err)
	}

	return &userv1.DeleteSubscriptionResponse{}, nil
}
