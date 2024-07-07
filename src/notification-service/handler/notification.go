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

	"github.com/wwi21seb-projekt/alpha-services/src/notification-service/dto"
	"github.com/wwi21seb-projekt/alpha-services/src/notification-service/schema"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	sq "github.com/Masterminds/squirrel"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
var vapidPrivateKey = os.Getenv("VAPID_PRIVATE_KEY")
var vapidPublicKey = os.Getenv("VAPID_PUBLIC_KEY")

type NotificationService struct {
	logger             *zap.SugaredLogger
	db                 *db.DB
	tracer             trace.Tracer
	profileClient      userv1.UserServiceClient
	subscriptionClient userv1.SubscriptionServiceClient
	notificationv1.UnimplementedNotificationServiceServer
}

func NewNotificationServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient userv1.UserServiceClient, subscriptionClient userv1.SubscriptionServiceClient) notificationv1.NotificationServiceServer {
	return &NotificationService{
		logger:             logger,
		db:                 db,
		tracer:             otel.GetTracerProvider().Tracer("notification-service"),
		profileClient:      profileClient,
		subscriptionClient: subscriptionClient,
	}
}

func (n *NotificationService) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	dataQuery, dataArgs, _ := psql.
		Select("*").
		From("notifications").
		Where("recipient_username = ?", authenticatedUsername).
		ToSql()

	conn, err := n.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		n.logger.Errorf("Error executing query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error executing query: %v", err)
	}
	defer rows.Close()

	notifications, err := pgx.CollectRows(rows, pgx.RowToStructByName[schema.Notification])
	if err != nil {
		n.logger.Errorf("Error collecting rows: %v", err)
		return nil, status.Error(codes.Internal, "Error collecting rows")
	}

	// Get the sender map
	senderMap, err := n.getSenderMap(ctx, notifications)
	if err != nil {
		n.logger.Errorf("Error getting sender map: %v", err)
		return nil, status.Error(codes.Internal, "Error getting sender map")
	}

	notificationsProto := make([]*notificationv1.Notification, 0, len(notifications))
	for _, notification := range notifications {
		notificationsProto = append(notificationsProto, notification.ToProto(senderMap))
	}

	resp := &notificationv1.ListNotificationsResponse{Notifications: notificationsProto}
	return resp, nil
}

func (n *NotificationService) getSenderMap(ctx context.Context, notifications []schema.Notification) (map[string]*userv1.User, error) {
	senderNames := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		senderNames = append(senderNames, notification.SenderUsername)
	}

	authorsCTX, authorsSpan := n.tracer.Start(ctx, "Fetch user data of senders")
	authorProfiles, err := n.profileClient.ListUsers(authorsCTX, &userv1.ListUsersRequest{Usernames: senderNames})
	if err != nil {
		authorsSpan.End()
		n.logger.Errorw("error getting user data", "error", err)
		return nil, status.Error(codes.Internal, "Error getting user data")
	}
	authorsSpan.End()

	senderMap := make(map[string]*userv1.User)
	for _, profile := range authorProfiles.GetUsers() {
		senderMap[profile.GetUsername()] = profile
	}

	return senderMap, nil
}

func (n *NotificationService) DeleteNotification(ctx context.Context, request *notificationv1.DeleteNotificationRequest) (*notificationv1.DeleteNotificationResponse, error) {
	conn, err := n.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := n.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer n.db.RollbackTx(ctx, tx)

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
	if err = n.db.CommitTx(ctx, tx); err != nil {
		n.logger.Errorf("Error in n.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in n.db.Commit: %v", err)
	}
	return &notificationv1.DeleteNotificationResponse{}, nil
}

func (n *NotificationService) SendNotification(ctx context.Context, request *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	// Fetch the username of the authenticated user (who is sending the notification)
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	notification := schema.Notification{
		NotificationID:    uuid.New(),
		RecipientUsername: request.GetRecipient(),
		NotificationType:  request.NotificationType,
		SenderUsername:    authenticatedUsername,
		Timestamp:         time.Now(),
	}

	query, args, _ := psql.Insert("notifications").
		Columns("notification_id", "recipient_username", "sender_username", "timestamp", "notification_type").
		Values(notification.NotificationID, notification.RecipientUsername, notification.SenderUsername, notification.Timestamp, notification.NotificationType).
		ToSql()

	conn, err := n.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := n.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer n.db.RollbackTx(ctx, tx)

	if _, err = tx.Exec(ctx, query, args...); err != nil {
		n.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	// Check if the recipient of the notification has any subscriptions
	subscriptionsQuery, subscriptionsArgs, _ := psql.
		Select("*").
		From("push_subscriptions").
		Where("username = ?", notification.RecipientUsername).
		ToSql()

	subscriptionRows, err := tx.Query(ctx, subscriptionsQuery, subscriptionsArgs...)
	if err != nil {
		n.logger.Errorf("Error executing query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error executing query: %v", err)
	}
	defer subscriptionRows.Close()

	subscriptions, err := pgx.CollectRows(subscriptionRows, pgx.RowToStructByName[schema.PushSubscription])
	if err != nil {
		n.logger.Errorw("Error collecting push subscription rows", "error", err)
		return nil, status.Error(codes.Internal, "Error collecting push subscription rows")
	}

	for _, subscription := range subscriptions {
		// Send notification based on device type
		switch subscription.Type {
		case "expo":
			// Send notification to Expo
			err = sendExpoNotification(ctx, request.NotificationType, *subscription.Token)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Error sending Expo notification: %v", err)
			}
		case "web":
			// Send notification to web
			err = n.sendWebNotification(ctx, &notification, &subscription)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Error sending web notification: %v", err)
			}
		default:
			return nil, status.Errorf(codes.Internal, "Unknown device type: %s", subscription.Type)
		}

	}

	// Commit transaction
	if err = n.db.CommitTx(ctx, tx); err != nil {
		n.logger.Errorf("Error in n.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in n.db.Commit: %v", err)
	}

	return nil, nil
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
	case "chat":
		title := "New Message!"
		body := fmt.Sprintf("%s sent you a message", authenticatedUsername)
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

func (n *NotificationService) sendWebNotification(ctx context.Context, notification *schema.Notification, pushSubscription *schema.PushSubscription) error {
	// Check if expiration time is valid and if it is in the past
	if pushSubscription.ExpirationTime != nil && pushSubscription.ExpirationTime.Before(time.Now()) {
		n.logger.Errorw("subscription expired", "expirationTime", pushSubscription.ExpirationTime)
		return status.Errorf(codes.InvalidArgument, "subscription expired")
	}

	// Fetch User
	users, err := n.profileClient.ListUsers(ctx, &userv1.ListUsersRequest{
		Usernames: []string{notification.SenderUsername},
	})
	if err != nil {
		n.logger.Errorw("error getting user", "error", err)
		return status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	if len(users.GetUsers()) == 0 {
		return status.Errorf(codes.NotFound, "user not found")
	}
	user := users.GetUsers()[0]

	notificationDTO := dto.Notification{
		NotificationType: notification.NotificationType,
		NotificationID:   notification.NotificationID.String(),
		User: dto.User{
			Username: user.GetUsername(),
			Nickname: user.GetNickname(),
		},
	}

	if user.GetPicture() != nil {
		notificationDTO.User.Picture = &dto.Picture{
			Url:    user.GetPicture().GetUrl(),
			Width:  user.GetPicture().GetWidth(),
			Height: user.GetPicture().GetHeight(),
		}
	}

	notificationPayloadBytes, err := json.Marshal(notificationDTO)
	if err != nil {
		n.logger.Errorw("failed to marshal notification payload", "error", err)
		return status.Errorf(codes.Internal, "failed to marshal notification payload: %v", err)
	}

	sub := &webpush.Subscription{
		Endpoint: *pushSubscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: *pushSubscription.P256dh,
			Auth:   *pushSubscription.Auth,
		},
	}

	sendCTX, sendSpan := n.tracer.Start(ctx, "Send Web Notification")
	n.logger.Debugw("sending web notification", "notification", notificationDTO, "subscription", sub, "vapidPublicKey", vapidPublicKey)
	resp, err := webpush.SendNotificationWithContext(sendCTX, notificationPayloadBytes, sub, &webpush.Options{
		TTL:             300,
		VAPIDPublicKey:  vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
	})
	if err != nil {
		sendSpan.End()
		n.logger.Errorw("failed to send notification", "error", err)
		return status.Errorf(codes.Internal, "failed to send notification: %v", err)
	}
	sendSpan.End()
	defer resp.Body.Close()

	return nil
}
