-- Add new schema named "notification_service"
CREATE SCHEMA "notification_service";
-- Set comment to schema: "notification_service"
COMMENT ON SCHEMA "notification_service" IS 'Notification service schema';
-- Create "push_subscriptions" table
CREATE TABLE "notification_service"."push_subscriptions" ("subscription_id" uuid NOT NULL, "username" character varying(25) NOT NULL, "type" character varying(4) NOT NULL, "token" character varying(64) NULL, "endpoint" character varying(256) NULL, "expiration_time" timestamptz NULL, "p256dh" character varying(256) NULL, "auth" character varying(256) NULL, PRIMARY KEY ("subscription_id"));
-- Create "notifications" table
CREATE TABLE "notification_service"."notifications" ("notification_id" uuid NOT NULL, "recipient_username" character varying(25) NOT NULL, "notification_type" character varying(6) NOT NULL, "sender_username" character varying(25) NOT NULL, "timestamp" timestamptz NOT NULL, PRIMARY KEY ("notification_id"), CONSTRAINT "recipient_fk" FOREIGN KEY ("recipient_username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "sender_fk" FOREIGN KEY ("sender_username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
