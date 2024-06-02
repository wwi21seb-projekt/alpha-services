package handler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbNotification "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type subscriptionService struct {
	db                 *db.DB
	notificationClient pbNotification.NotificationServiceClient
	pb.UnimplementedSubscriptionServiceServer
}

func NewSubscriptionServer(database *db.DB, notificiationClient pbNotification.NotificationServiceClient) pb.SubscriptionServiceServer {
	return &subscriptionService{
		db:                 database,
		notificationClient: notificiationClient,
	}
}

func (ss subscriptionService) ListSubscriptions(ctx context.Context, request *pb.ListSubscriptionsRequest) (*pb.ListSubscriptionsResponse, error) {
	conn, err := ss.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("Error in ss.db.Pool.Acquire: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Get the followers by default
	userTypes := []string{"subscriber_name", "subscribee_name"}
	// If the subscription type is following, fetch the users the user is following
	if request.GetSubscriptionType() == pb.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING {
		userTypes = []string{"subscribee_name", "subscriber_name"}
	}

	// For every subscription we also want to know if the authenticated user is subscribed to the subscribee and vice
	// versa. Table S1 is the current subscriber or subscribee in the loop of the user we are querying, S2 represents
	// the subscription of the current user in the loop to the authenticated user and S3 the other way around.
	dataQuery, dataArgs, _ := psql.Select().
		Columns("s2.subscription_id", "s3.subscription_id").
		Columns("u.username", "u.nickname", "u.profile_picture_url").
		From("users u").
		Join("subscriptions s1 ON s1."+userTypes[0]+" = u.username").
		LeftJoin("subscriptions s2 ON s2.subscriber_name = u.username AND s2.subscribee_name = ?", authenticatedUsername).
		LeftJoin("subscriptions s3 ON s3.subscriber_name = ? AND s3.subscribee_name = u.username", authenticatedUsername).
		Where("s1."+userTypes[1]+" = ?", request.GetUsername()).
		GroupBy("s1.created_at", "s2.subscription_id", "s3.subscription_id", "u.username", "u.nickname", "u.profile_picture_url").
		OrderBy("s1.created_at DESC").
		Limit(uint64(request.GetPagination().GetLimit())).
		Offset(uint64(request.GetPagination().GetOffset())).
		ToSql()

	// Count the total number of subscriptions and also validate that the user exists
	countQuery, countArgs, _ := psql.Select("COUNT(s.subscription_id)", "COUNT(u.username)").
		From("users u").
		// We need to use a left join here in case there's no subscriptions to list
		LeftJoin("subscriptions s ON s."+userTypes[1]+" = u.username").
		Where("u.username = ?", request.GetUsername()).
		ToSql()

	log.Info("Starting batch request for subscription data")
	batch := &pgx.Batch{}
	batch.Queue(countQuery, countArgs...)
	batch.Queue(dataQuery, dataArgs...)
	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	// Get the first batch result which contains the total number of subscriptions and the user count
	var subscriptionCount, userCount int32
	if err = br.QueryRow().Scan(&subscriptionCount, &userCount); err != nil {
		log.Errorf("Error in br.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in br.QueryRow: %v", err)
	}

	if userCount == 0 {
		log.Errorf("user %s does not exist", request.GetUsername())
		return nil, status.Errorf(codes.NotFound, "user does not exist")
	}

	// Get the second batch result which contains the subscription data
	rows, err := br.Query()
	if err != nil {
		log.Errorf("Error in br.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in br.Query: %v", err)
	}
	defer rows.Close()

	response := &pb.ListSubscriptionsResponse{}
	for rows.Next() {
		subscription := &pb.Subscription{}
		var followedId, followerId, nickname, profilePictureUrl pgtype.Text

		if err = rows.Scan(&followedId, &followerId, &subscription.Username, &nickname, &profilePictureUrl); err != nil {
			log.Errorf("Error in rows.Scan: %v", err)
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
		if profilePictureUrl.Valid {
			subscription.ProfilePictureUrl = profilePictureUrl.String
		}
		response.Subscriptions = append(response.Subscriptions, subscription)
	}

	response.Pagination = &pbCommon.Pagination{
		Records: subscriptionCount,
		Offset:  request.GetPagination().GetOffset(),
		Limit:   request.GetPagination().GetLimit(),
	}

	return response, nil
}

func (ss subscriptionService) CreateSubscription(ctx context.Context, request *pb.CreateSubscriptionRequest) (*pb.CreateSubscriptionResponse, error) {
	tx, err := ss.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in ss.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer ss.db.Rollback(ctx, tx)

	// Fetch the username of the authenticated user
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Return early error if the user tries to subscribe to themselves
	if username == request.GetFollowedUsername() {
		log.Errorf("user %s tried to subscribe to themselves", username)
		return nil, status.Errorf(codes.InvalidArgument, "user %s tried to subscribe to themselves", username)
	}

	// Create the subscription within the transaction, we'll get a constraint violation if the
	// user does not exist or the subscription already exists
	subscriptionId := uuid.New()
	createdAt := time.Now()
	query, args, _ := psql.Insert("subscriptions").
		Columns("subscription_id", "subscriber_name", "subscribee_name", "created_at").
		Values(subscriptionId, username, request.GetFollowedUsername(), createdAt).
		ToSql()

	log.Info("Creating subscription")
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		// Check if the error is a constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
			// We need to differentiate between the two possible constraint violations
			// 1: Foreign key violation -> user does not exist
			// 2: Unique violation -> subscription already exists
			if pgErr.ConstraintName == "subscribee_fk" {
				log.Errorf("user %s does not exist", request.GetFollowedUsername())
				return nil, status.Error(codes.NotFound, "user does not exist")
			} else if pgErr.ConstraintName == "subscriptions_uq" {
				log.Errorf("subscription already exists")
				return nil, status.Errorf(codes.AlreadyExists, "subscription already exists")
			}
		}

		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	if err = ss.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in ss.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in ss.db.Commit: %v", err)
	}

	// Send a notification to the user that they have been subscribed to
	sendNotificationRequest := pbNotification.SendNotificationRequest{
		NotificationType: "follow",
		Sender:           request.GetFollowedUsername(),
	}

	if _, err = ss.notificationClient.SendNotification(ctx, &sendNotificationRequest); err != nil {
		log.Errorf("Error in ss.notificationClient.SendNotification: %v", err)
	}

	return &pb.CreateSubscriptionResponse{
		SubscriptionId:   subscriptionId.String(),
		SubscriptionDate: createdAt.Format(time.RFC3339),
		FollowerUsername: username,
		FollowedUsername: request.GetFollowedUsername(),
	}, nil
}
func (ss subscriptionService) DeleteSubscription(ctx context.Context, request *pb.DeleteSubscriptionRequest) (*pbCommon.Empty, error) {
	tx, err := ss.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in ss.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer ss.db.Rollback(ctx, tx)

	// Fetch the username of the authenticated user
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Trigger the deletion of the subscription within the transaction, if the subscription does not exist
	// we'll get an appropriate error
	query, args, _ := psql.Delete("subscriptions").
		Where("subscription_id = ?", request.GetSubscriptionId()).
		// We return the subscriber_name to verify that the authenticated user is the subscriber
		Suffix("RETURNING subscriber_name").
		ToSql()

	log.Info("Deleting subscription...")
	var subscriberName string
	if err = tx.QueryRow(ctx, query, args...).Scan(&subscriberName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Errorf("subscription %s does not exist", request.GetSubscriptionId())
			return nil, status.Errorf(codes.NotFound, "subscription does not exist")
		}

		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.QueryRow: %v", err)
	}

	// Check if the authenticated user is the subscriber
	if subscriberName != username {
		log.Errorf("user %s tried to delete subscription %s", username, request.GetSubscriptionId())
		return nil, status.Errorf(codes.PermissionDenied, "the authenticated user is not the subscriber")
	}

	if err = ss.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in ss.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in ss.db.Commit: %v", err)
	}

	return &pbCommon.Empty{}, nil
}
