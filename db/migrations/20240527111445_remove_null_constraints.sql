-- Modify "users" table
ALTER TABLE "user_service"."users" ALTER COLUMN "status" DROP NOT NULL, ALTER COLUMN "profile_picture_url" DROP NOT NULL;
