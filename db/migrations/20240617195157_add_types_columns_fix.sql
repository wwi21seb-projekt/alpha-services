-- Modify "notifications" table
ALTER TABLE "notification_service"."notifications" ADD COLUMN "notification_type" "notification_service"."notification_type" NOT NULL;
-- Modify "push_subscriptions" table
ALTER TABLE "notification_service"."push_subscriptions" ADD COLUMN "type" "notification_service"."subscription_type" NOT NULL;
