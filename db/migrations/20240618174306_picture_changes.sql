-- Modify "posts" table
ALTER TABLE "post_service"."posts" ADD COLUMN "picture_url" character varying(64), ADD COLUMN "picture_width" integer, ADD COLUMN "picture_height" integer;
-- Modify "users" table
ALTER TABLE "user_service"."users" DROP COLUMN "profile_picture_url", ADD COLUMN "picture_url" character varying(64) NOT NULL, ADD COLUMN "picture_width" integer NOT NULL, ADD COLUMN "picture_height" integer NOT NULL;
