package schema

// ----------------- Request Schemas -----------------

type CreatePushSubscriptionRequest struct {
	Type         string           `json:"type"`                   // "web" or "expo"
	Token        string           `json:"token,omitempty"`        // only for expo
	Subscription *WebSubscription `json:"subscription,omitempty"` // only for web
}

type WebSubscription struct {
	Endpoint       string              `json:"endpoint"`
	ExpirationTime *string             `json:"expirationTime,omitempty"` // optional
	Keys           WebSubscriptionKeys `json:"keys"`
}

type WebSubscriptionKeys struct {
	P256Dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// ----------------- Response Schemas -----------------

type GetPublicKeyResponse struct {
	Key string `json:"key"`
}

type CreatePushSubscriptionResponse struct {
	SubscriptionID string `json:"subscriptionId"`
}
