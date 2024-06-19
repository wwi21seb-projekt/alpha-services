-- Create enum type "subscription_type"
CREATE TYPE "notification_service"."subscription_type" AS ENUM ('expo', 'web');
-- Create enum type "notification_type"
CREATE TYPE "notification_service"."notification_type" AS ENUM ('follow', 'repost');
-- Modify "notifications" table
ALTER TABLE "notification_service"."notifications" DROP COLUMN "notification_type";
-- Modify "push_subscriptions" table
ALTER TABLE "notification_service"."push_subscriptions" DROP COLUMN "type", ADD CONSTRAINT "username_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
