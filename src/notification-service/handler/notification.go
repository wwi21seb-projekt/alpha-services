package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
var vapidPrivateKey = os.Getenv("VAPID_PRIVATE_KEY")
var vapidPublicKey = os.Getenv("VAPID_PUBLIC_KEY")

type NotificationService struct {
	logger             *zap.SugaredLogger
	db                 *db.DB
	profileClient      pbUser.UserServiceClient
	subscriptionClient pbUser.SubscriptionServiceClient
	pb.UnimplementedNotificationServiceServer
}

func NewNotificationServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient, subscriptionClient pbUser.SubscriptionServiceClient) pb.NotificationServiceServer {
	return &NotificationService{
		logger:             logger,
		db:                 db,
		profileClient:      profileClient,
		subscriptionClient: subscriptionClient,
	}
}

func (n *NotificationService) GetNotifications(ctx context.Context, _ *pbCommon.Empty) (*pb.GetNotificationsResponse, error) {
	conn, err := n.db.Pool.Acquire(ctx)
	if err != nil {
		n.logger.Errorf("Error in n.db.Pool.Acquire: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	dataQuery, dataArgs, _ := psql.Select().
		Columns("n.notification_id", "n.timestamp", "n.notification_type", "n.sender_username").
		From("notifications n").
		Where("n.recipient_username = ?", authenticatedUsername).
		ToSql()

	n.logger.Info("Executing single query for notification data")
	rows, err := conn.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		n.logger.Errorf("Error executing query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error executing query: %v", err)
	}
	defer rows.Close()

	var usernames []string

	notificationsResponse := &pb.GetNotificationsResponse{}
	for rows.Next() {
		var username, notificationId, notificationType pgtype.Text
		var timestamp pgtype.Timestamptz

		if err = rows.Scan(&notificationId, &timestamp, &notificationType, &username); err != nil {
			n.logger.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Errorf(codes.Internal, "Error in rows.Scan: %v", err)
		}
		usernames = append(usernames, username.String)

		var notificationTypeResponse pb.NotificationType
		if notificationType.String == "follow" {
			notificationTypeResponse = pb.NotificationType_FOLLOW
		} else {
			notificationTypeResponse = pb.NotificationType_REPOST
		}
		user := &pbUser.User{
			Username: username.String,
		}

		notification := &pb.Notification{
			NotificationId:  notificationId.String,
			Timestamp:       timestamp.Time.Format(time.RFC3339),
			NotficationType: notificationTypeResponse,
			User:            user,
		}

		notificationsResponse.Notifications = append(notificationsResponse.Notifications, notification)
	}

	userdata, err := n.profileClient.ListUsers(ctx, &pbUser.ListUsersRequest{Usernames: usernames})
	if err != nil {
		n.logger.Errorf("Error in n.profileClient.ListUsers: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to get users: %v", err)
	}

	userMap := make(map[string]*pbUser.User)
	for _, user := range userdata.Users {
		userMap[user.Username] = &pbUser.User{
			Nickname: user.Nickname,
			Picture:  user.Picture,
		}
	}
	for _, notfication := range notificationsResponse.Notifications {

		notfication.User.Nickname = userMap[notfication.User.Username].Nickname
		notfication.User.Picture = userMap[notfication.User.Username].Picture

	}
	return notificationsResponse, nil
}

func (n *NotificationService) DeleteNotification(ctx context.Context, request *pb.DeleteNotificationRequest) (*pbCommon.Empty, error) {
	tx, err := n.db.Begin(ctx)
	if err != nil {
		n.logger.Errorf("Error in n.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer n.db.Rollback(ctx, tx)

	query, args, _ := psql.Delete("notifications").
		Where("notification_id = ?", request.GetNotificationId()).
		// We return the notification to verify that the given notification exists
		Suffix("RETURNING notification_id").
		ToSql()

	n.logger.Info("Deleting notification...")
	var notificationId string
	if err := tx.QueryRow(ctx, query, args...).Scan(&notificationId); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			n.logger.Errorf("notification %s does not exist", request.GetNotificationId())
			return nil, status.Errorf(codes.NotFound, "notification does not exist")
		}

		n.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.QueryRow: %v", err)
	}
	if err = n.db.Commit(ctx, tx); err != nil {
		n.logger.Errorf("Error in n.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in n.db.Commit: %v", err)
	}
	return &pbCommon.Empty{}, nil
}

func (n *NotificationService) SendNotification(ctx context.Context, request *pb.SendNotificationRequest) (*pbCommon.Empty, error) {
	tx, err := n.db.Begin(ctx)
	if err != nil {
		n.logger.Errorf("Error in n.db.Begin: %v", err)
		return &pbCommon.Empty{}, status.Errorf(codes.Internal, "could not start transaction: %v", err)
	}
	defer n.db.Rollback(ctx, tx)

	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	query, args, _ := psql.Insert("notifications").
		Columns("notification_id", "recipient_username", "sender_username", "timestamp", "notification_type").
		Values(uuid.New(), request.Recipient, authenticatedUsername, time.Now(), request.NotificationType).
		ToSql()

	if _, err = tx.Exec(ctx, query, args...); err != nil {
		n.logger.Errorf("Error in tx.Exec: %v", err)
		return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	// Check if the recipient of the notification has any subscriptions
	subscriptionsQuery, subscriptionsArgs, _ := psql.Select().
		Columns("s.subscription_id", "s.type", "s.token", "s.endpoint", "s.expiration_time", "s.p256dh", "s.auth").
		From("push_subscriptions s").
		Where("s.username = ?", request.Recipient).
		ToSql()
	subscriptionRows, err := tx.Query(ctx, subscriptionsQuery, subscriptionsArgs...)
	if err != nil {
		n.logger.Errorf("Error executing query: %v", err)
		return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Error executing query: %v", err)
	}
	defer subscriptionRows.Close()

	// Iterate over the subscription rows
	for subscriptionRows.Next() {
		var subscriptionID, deviceType, token, endpoint, p256dh, auth pgtype.Text
		var expirationTime pgtype.Timestamptz

		if err = subscriptionRows.Scan(&subscriptionID, &deviceType, &token, &endpoint, &expirationTime, &p256dh, &auth); err != nil {
			n.logger.Errorf("Error in subscriptionRows.Scan: %v", err)
			return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Error in subscriptionRows.Scan: %v", err)
		}
		// Send notification based on device type
		switch deviceType.String {
		case "expo":
			// Send notification to Expo
			err = sendExpoNotification(ctx, request.NotificationType, token.String)
			if err != nil {
				return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Error sending Expo notification: %v", err)
			}
		case "web":
			// Send notification to web
			err = sendWebNotification(ctx, request.NotificationType, endpoint.String, expirationTime, p256dh.String, auth.String)
			if err != nil {
				return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Error sending web notification: %v", err)
			}
		default:
			return &pbCommon.Empty{}, status.Errorf(codes.Internal, "Unknown device type: %s", deviceType.String)
		}
	}

	return &pbCommon.Empty{}, nil
}

func sendExpoNotification(ctx context.Context, notificationType string, token string) error {
	// Send Expo notification
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	data := make(map[string]interface{})
	switch notificationType {
	case "follow":
		title := "New Follower!"
		body := fmt.Sprintf("%s started following you", authenticatedUsername)
		data = map[string]interface{}{
			"to":    token,
			"title": title,
			"body":  body,
		}
	case "repost":
		title := "New Repost!"
		body := fmt.Sprintf("%s reposted your post", authenticatedUsername)
		data = map[string]interface{}{
			"to":    token,
			"title": title,
			"body":  body,
		}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to marshal notification payload: %v", err)
	}
	resp, err := http.Post("https://exp.host/--/api/v2/push/send", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return status.Errorf(codes.Internal, "failed to send notification: %v", err)
	}
	defer resp.Body.Close()
	return nil
}

func sendWebNotification(ctx context.Context, notificationType string, endpoint string, expirationTime pgtype.Timestamptz, p256dh string, auth string) error {
	// Check if expiration time is in the past
	if expirationTime.Time.Before(time.Now()) {
		return status.Errorf(codes.InvalidArgument, "subscription expired")
	}

	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	var title, body string

	switch notificationType {
	case "follow":
		title = "New Follower!"
		body = fmt.Sprintf("%s started following you", authenticatedUsername)
	case "repost":
		title = "New Repost!"
		body = fmt.Sprintf("%s reposted your post", authenticatedUsername)
	}

	notificationPayload := map[string]string{
		"title": title,
		"body":  body,
	}
	notificationPayloadBytes, err := json.Marshal(notificationPayload)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to marshal notification payload: %v", err)
	}

	sub := &webpush.Subscription{
		Endpoint: endpoint,
		Keys: webpush.Keys{
			P256dh: p256dh,
			Auth:   auth,
		},
	}

	resp, err := webpush.SendNotification(notificationPayloadBytes, sub, &webpush.Options{
		TTL:             300,
		VAPIDPublicKey:  vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
	})
	if err != nil {
		return status.Errorf(codes.Internal, "failed to send notification: %v", err)
	}
	defer resp.Body.Close()

	return nil
}
