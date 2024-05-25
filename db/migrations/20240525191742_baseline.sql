-- Drop schema named "public"
DROP SCHEMA "public" CASCADE;
-- Add new schema named "post_service"
CREATE SCHEMA "post_service";
-- Set comment to schema: "post_service"
COMMENT ON SCHEMA "post_service" IS 'Post service schema';
-- Add new schema named "user_service"
CREATE SCHEMA "user_service";
-- Set comment to schema: "user_service"
COMMENT ON SCHEMA "user_service" IS 'User service schema';
-- Create "users" table
CREATE TABLE "user_service"."users" ("user_id" uuid NOT NULL, "username" character varying(25) NOT NULL, "nickname" character varying(20) NOT NULL, "email" character varying(128) NOT NULL, "password" character(60) NOT NULL, "status" character varying(256) NOT NULL, "profile_picture_url" character varying(256) NOT NULL, "created_at" timestamptz NOT NULL, "activated_at" timestamptz NULL, "expires_at" timestamptz NULL, PRIMARY KEY ("user_id"));
-- Create "activation_tokens" table
CREATE TABLE "user_service"."activation_tokens" ("token_id" uuid NOT NULL, "token" character varying(6) NOT NULL, "expires_at" timestamptz NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("token_id"), CONSTRAINT "users_fk" FOREIGN KEY ("user_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "posts" table
CREATE TABLE "post_service"."posts" ("post_id" uuid NOT NULL, "content" character varying(256) NULL, "created_at" timestamptz NOT NULL, "author_id" uuid NOT NULL, "longitude" double precision NULL, "latitude" double precision NULL, "accuracy" integer NULL, "repost_post_id" uuid NULL, PRIMARY KEY ("post_id"), CONSTRAINT "users_fk" FOREIGN KEY ("author_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "comments" table
CREATE TABLE "post_service"."comments" ("comment_id" uuid NOT NULL, "content" character varying(128) NOT NULL, "created_at" timestamptz NOT NULL, "author_id" uuid NOT NULL, "post_id" uuid NOT NULL, PRIMARY KEY ("comment_id"), CONSTRAINT "posts_fk" FOREIGN KEY ("post_id") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("author_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "likes" table
CREATE TABLE "post_service"."likes" ("user_id" uuid NOT NULL, "post_id" uuid NOT NULL, "liked_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "post_id"), CONSTRAINT "posts_fk" FOREIGN KEY ("post_id") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("user_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "hashtags" table
CREATE TABLE "post_service"."hashtags" ("hashtag_id" uuid NOT NULL, "content" character varying(32) NOT NULL, PRIMARY KEY ("hashtag_id"), CONSTRAINT "hashtags_uq" UNIQUE ("content"));
-- Create "many_posts_has_many_hashtags" table
CREATE TABLE "post_service"."many_posts_has_many_hashtags" ("post_id_posts" uuid NOT NULL, "hashtag_id_hashtags" uuid NOT NULL, PRIMARY KEY ("post_id_posts", "hashtag_id_hashtags"), CONSTRAINT "hashtags_fk" FOREIGN KEY ("hashtag_id_hashtags") REFERENCES "post_service"."hashtags" ("hashtag_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "posts_fk" FOREIGN KEY ("post_id_posts") REFERENCES "post_service"."posts" ("post_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Create "subscriptions" table
CREATE TABLE "user_service"."subscriptions" ("subscription_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "subscriber_id" uuid NOT NULL, "subscribee_id" uuid NOT NULL, PRIMARY KEY ("subscription_id"), CONSTRAINT "subscriptions_uq" UNIQUE ("subscriber_id", "subscribee_id"), CONSTRAINT "subscribee_fk" FOREIGN KEY ("subscribee_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "subscriber_fk" FOREIGN KEY ("subscriber_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
