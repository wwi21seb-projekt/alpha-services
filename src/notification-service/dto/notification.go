package dto

type Notification struct {
	NotificationID   string `json:"notificationId"`
	Timestamp        string `json:"timestamp"`
	NotificationType string `json:"notificationType"`
	User             User   `json:"user"`
}

type User struct {
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Picture  *Picture `json:"picture,omitempty"`
}

type Picture struct {
	Url    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}
