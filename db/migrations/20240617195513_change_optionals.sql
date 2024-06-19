-- Modify "posts" table
ALTER TABLE "post_service"."posts" ALTER COLUMN "content" SET NOT NULL;
-- Modify "users" table
ALTER TABLE "user_service"."users" ALTER COLUMN "profile_picture_url" SET NOT NULL;
