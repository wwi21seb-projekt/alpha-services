package schema

type GetNotificationsResponse struct {
	Records []Notification `json:"records"`
}

type Notification struct {
	NotificationID   string `json:"notificationId"`
	Timestamp        string `json:"timestamp"`
	NotificationType string `json:"notificationType"`
	User             Author `json:"user"`
}

type GetPublicKeyResponse struct {
	Key string `json:"key"`
}

type CreatePushSubscriptionResponse struct {
	SubscriptionID string `json:"subscriptionId"`
}
