-- Add new schema named "post_service"
CREATE SCHEMA "post_service";
-- Set comment to schema: "post_service"
COMMENT ON SCHEMA "post_service" IS 'Post service schema';
-- Add new schema named "user_service"
CREATE SCHEMA "user_service";
-- Set comment to schema: "user_service"
COMMENT ON SCHEMA "user_service" IS 'User service schema';
-- Create "users" table
CREATE TABLE "user_service"."users" ("id" integer NOT NULL, "username" character varying(50) NOT NULL, "email" character varying(100) NOT NULL, PRIMARY KEY ("id"));
-- Create "posts" table
CREATE TABLE "post_service"."posts" ("id" integer NOT NULL, "user_id" integer NOT NULL, "title" character varying(100) NOT NULL, "content" text NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "fk_user_id" FOREIGN KEY ("user_id") REFERENCES "user_service"."users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create "likes" table
CREATE TABLE "post_service"."likes" ("id" integer NOT NULL, "user_id" integer NOT NULL, "post_id" integer NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "fk_post_id" FOREIGN KEY ("post_id") REFERENCES "post_service"."posts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "fk_user_id" FOREIGN KEY ("user_id") REFERENCES "user_service"."users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
