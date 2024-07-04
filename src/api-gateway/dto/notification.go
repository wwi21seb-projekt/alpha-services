package dto

import notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"

type ListNotificationResponse struct {
	Records []Notification `json:"records"`
}

type Notification struct {
	NotificationId   string `json:"notificationId"`
	Timestamp        string `json:"timestamp"`
	NotificationType string `json:"notificationType"`
	User             User   `json:"user"`
}

func TransformNotificationProtoToDTO(proto *notificationv1.ListNotificationsResponse) ListNotificationResponse {
	notificationDTOs := make([]Notification, 0, len(proto.GetNotifications()))

	for _, notification := range proto.GetNotifications() {
		notificationDTO := Notification{
			NotificationId: notification.GetNotificationId(),
			Timestamp:      notification.GetTimestamp(),
			User:           TransformProtoUserToDTO(notification.GetUser()),
		}

		switch notification.GetNotificationType() {
		case notificationv1.NotificationType_NOTIFICATION_TYPE_FOLLOW:
			notificationDTO.NotificationType = "follow"
		case notificationv1.NotificationType_NOTIFICATION_TYPE_REPOST:
			notificationDTO.NotificationType = "repost"
		case notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT:
			notificationDTO.NotificationType = "message"
		}

		notificationDTOs = append(notificationDTOs, notificationDTO)
	}

	return ListNotificationResponse{
		Records: notificationDTOs,
	}
}
