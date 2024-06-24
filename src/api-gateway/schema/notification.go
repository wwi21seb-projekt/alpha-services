package schema

type Notification struct {
	NotificationID   string `json:"notificationId"`
	Timestamp        string `json:"timestamp"`
	NotificationType string `json:"notificationType"`
	User             Author `json:"user"`
}

// ----------------- Request Schemas -----------------

type CreatePushSubscriptionRequest struct {
	Endpoint       string `json:"endpoint"`
	P256Dh         string `json:"p256dh"`
	Auth           string `json:"auth"`
	Type           string `json:"type"`
	Token          string `json:"token"`
	ExpirationTime string `json:"expirationTime"`
}

type GetNotificationsResponse struct {
	Records []Notification `json:"records"`
}

// ----------------- Response Schemas -----------------

type GetPublicKeyResponse struct {
	Key string `json:"key"`
}

type CreatePushSubscriptionResponse struct {
	SubscriptionID string `json:"subscriptionId"`
}
