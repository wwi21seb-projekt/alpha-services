-- Add new schema named "chat_service"
CREATE SCHEMA "chat_service";
-- Set comment to schema: "chat_service"
COMMENT ON SCHEMA "chat_service" IS 'Chat service schema';
-- Add new schema named "notification_service"
CREATE SCHEMA "notification_service";
-- Set comment to schema: "notification_service"
COMMENT ON SCHEMA "notification_service" IS 'Notification service schema';
-- Add new schema named "post_service"
CREATE SCHEMA "post_service";
-- Set comment to schema: "post_service"
COMMENT ON SCHEMA "post_service" IS 'Post service schema';
-- Add new schema named "user_service"
CREATE SCHEMA "user_service";
-- Set comment to schema: "user_service"
COMMENT ON SCHEMA "user_service" IS 'User service schema';
-- Create extension "fuzzystrmatch"
CREATE EXTENSION "fuzzystrmatch" WITH SCHEMA "user_service" VERSION "1.2";
-- Create enum type "token_type"
CREATE TYPE "user_service"."token_type" AS ENUM ('activation', 'password_reset');
-- Create "users" table
CREATE TABLE "user_service"."users" ("username" character varying(25) NOT NULL, "nickname" character varying(20) NOT NULL, "email" character varying(128) NOT NULL, "password" character(60) NOT NULL, "status" character varying(256) NULL, "picture_url" character varying(64) NOT NULL, "picture_width" integer NOT NULL, "picture_height" integer NOT NULL, "created_at" timestamptz NOT NULL, "activated_at" timestamptz NULL, "expires_at" timestamptz NULL, PRIMARY KEY ("username"), CONSTRAINT "email_uq" UNIQUE ("email"));
-- Create enum type "subscription_type"
CREATE TYPE "notification_service"."subscription_type" AS ENUM ('expo', 'web');
-- Create enum type "notification_type"
CREATE TYPE "notification_service"."notification_type" AS ENUM ('follow', 'repost');
-- Create "chats" table
CREATE TABLE "chat_service"."chats" ("chat_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "user1_name" character varying(25) NOT NULL, "user2_name" character varying(25) NOT NULL, PRIMARY KEY ("chat_id"), CONSTRAINT "users_uq" UNIQUE ("user1_name", "user2_name"), CONSTRAINT "user1_fk" FOREIGN KEY ("user1_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "user2_fk" FOREIGN KEY ("user2_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "chats_check" CHECK ((user1_name)::text < (user2_name)::text));
-- Create "posts" table
CREATE TABLE "post_service"."posts" ("post_id" uuid NOT NULL, "content" character varying(256) NOT NULL, "created_at" timestamptz NOT NULL, "author_name" character varying(25) NOT NULL, "longitude" double precision NULL, "latitude" double precision NULL, "accuracy" integer NULL, "picture_url" character varying(96) NULL, "picture_width" integer NULL, "picture_height" integer NULL, "repost_post_id" uuid NULL, PRIMARY KEY ("post_id"), CONSTRAINT "users_fk" FOREIGN KEY ("author_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "comments" table
CREATE TABLE "post_service"."comments" ("comment_id" uuid NOT NULL, "content" character varying(128) NOT NULL, "created_at" timestamptz NOT NULL, "author_name" character varying(25) NOT NULL, "post_id" uuid NOT NULL, PRIMARY KEY ("comment_id"), CONSTRAINT "posts_fk" FOREIGN KEY ("post_id") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("author_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "likes" table
CREATE TABLE "post_service"."likes" ("username" character varying(25) NOT NULL, "post_id" uuid NOT NULL, "liked_at" timestamptz NOT NULL, PRIMARY KEY ("username", "post_id"), CONSTRAINT "posts_fk" FOREIGN KEY ("post_id") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "hashtags" table
CREATE TABLE "post_service"."hashtags" ("hashtag_id" uuid NOT NULL, "content" character varying(32) NOT NULL, PRIMARY KEY ("hashtag_id"), CONSTRAINT "hashtags_uq" UNIQUE ("content"));
-- Create "many_posts_has_many_hashtags" table
CREATE TABLE "post_service"."many_posts_has_many_hashtags" ("post_id_posts" uuid NOT NULL, "hashtag_id_hashtags" uuid NOT NULL, PRIMARY KEY ("post_id_posts", "hashtag_id_hashtags"), CONSTRAINT "hashtags_fk" FOREIGN KEY ("hashtag_id_hashtags") REFERENCES "post_service"."hashtags" ("hashtag_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "posts_fk" FOREIGN KEY ("post_id_posts") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "messages" table
CREATE TABLE "chat_service"."messages" ("message_id" uuid NOT NULL, "content" character varying(256) NOT NULL, "created_at" timestamptz NOT NULL, "chat_id" uuid NOT NULL, "sender_name" character varying(25) NOT NULL, PRIMARY KEY ("message_id"), CONSTRAINT "chats_fk" FOREIGN KEY ("chat_id") REFERENCES "chat_service"."chats" ("chat_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("sender_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "notifications" table
CREATE TABLE "notification_service"."notifications" ("notification_id" uuid NOT NULL, "recipient_username" character varying(25) NOT NULL, "notification_type" "notification_service"."notification_type" NOT NULL, "sender_username" character varying(25) NOT NULL, "timestamp" timestamptz NOT NULL, PRIMARY KEY ("notification_id"), CONSTRAINT "recipient_fk" FOREIGN KEY ("recipient_username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "sender_fk" FOREIGN KEY ("sender_username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "push_subscriptions" table
CREATE TABLE "notification_service"."push_subscriptions" ("subscription_id" uuid NOT NULL, "username" character varying(25) NOT NULL, "type" "notification_service"."subscription_type" NOT NULL, "token" character varying(64) NULL, "endpoint" character varying(256) NULL, "expiration_time" timestamptz NULL, "p256dh" character varying(256) NULL, "auth" character varying(256) NULL, PRIMARY KEY ("subscription_id"), CONSTRAINT "username_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "subscriptions" table
CREATE TABLE "user_service"."subscriptions" ("subscription_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "subscriber_name" character varying(25) NOT NULL, "subscribee_name" character varying(25) NOT NULL, PRIMARY KEY ("subscription_id"), CONSTRAINT "subscriptions_uq" UNIQUE ("subscriber_name", "subscribee_name"), CONSTRAINT "subscribee_fk" FOREIGN KEY ("subscribee_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "subscriber_fk" FOREIGN KEY ("subscriber_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "tokens" table
CREATE TABLE "user_service"."tokens" ("token_id" uuid NOT NULL, "token" character varying(6) NOT NULL, "expires_at" timestamptz NOT NULL, "type" "user_service"."token_type" NOT NULL, "username" character varying(25) NOT NULL, PRIMARY KEY ("token_id"), CONSTRAINT "user_token_type_uq" UNIQUE ("username", "type"), CONSTRAINT "users_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
