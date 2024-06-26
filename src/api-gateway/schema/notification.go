package schema

// ----------------- Request Schemas -----------------

type CreatePushSubscriptionRequest struct {
	Endpoint       string `json:"endpoint"`
	P256Dh         string `json:"p256dh"`
	Auth           string `json:"auth"`
	Type           string `json:"type"`
	Token          string `json:"token"`
	ExpirationTime string `json:"expirationTime"`
}

// ----------------- Response Schemas -----------------

type GetPublicKeyResponse struct {
	Key string `json:"key"`
}

type CreatePushSubscriptionResponse struct {
	SubscriptionID string `json:"subscriptionId"`
}
