package schema

import (
	"github.com/google/uuid"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"time"
)

type Notification struct {
	NotificationID    uuid.UUID `db:"notification_id"`
	RecipientUsername string    `db:"recipient_username"`
	NotificationType  string    `db:"notification_type"`
	SenderUsername    string    `db:"sender_username"`
	Timestamp         time.Time `db:"timestamp"`
}

func (notification *Notification) ToProto(senderMap map[string]*userv1.User) *notificationv1.Notification {
	proto := &notificationv1.Notification{
		NotificationId: notification.NotificationID.String(),
		Timestamp:      notification.Timestamp.Format(time.RFC3339),
		User:           senderMap[notification.SenderUsername],
	}

	switch notification.NotificationType {
	case "follow":
		proto.NotificationType = notificationv1.NotificationType_NOTIFICATION_TYPE_FOLLOW
	case "repost":
		proto.NotificationType = notificationv1.NotificationType_NOTIFICATION_TYPE_REPOST
	case "message":
		proto.NotificationType = notificationv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}

	return proto
}
