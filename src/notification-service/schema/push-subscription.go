package schema

import (
	"github.com/google/uuid"
	"time"
)

// CREATE TABLE "notification_service"."push_subscriptions" (
// "subscription_id" uuid NOT NULL,
// "username" character varying(25) NOT NULL,
// "type" "notification_service"."subscription_type" NOT NULL,
// "token" character varying(64) NULL,
// "endpoint" character varying(256) NULL,
// "expiration_time" timestamptz NULL,
// "p256dh" character varying(256) NULL,
// "auth" character varying(256) NULL, PRIMARY KEY ("subscription_id"), CONSTRAINT "username_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);

type PushSubscription struct {
	SubscriptionID uuid.UUID  `db:"subscription_id"`
	Username       string     `db:"username"`
	Type           string     `db:"type"`
	Token          *string    `db:"token"`
	Endpoint       *string    `db:"endpoint"`
	ExpirationTime *time.Time `db:"expiration_time"`
	P256dh         *string    `db:"p256dh"`
	Auth           *string    `db:"auth"`
}
