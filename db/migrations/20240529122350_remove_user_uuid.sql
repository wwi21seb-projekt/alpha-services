-- Modify "users" table
ALTER TABLE "user_service"."users" DROP CONSTRAINT "users_pkey" CASCADE, DROP CONSTRAINT "username_uq", DROP COLUMN "user_id", ADD PRIMARY KEY ("username");
-- Modify "comments" table
ALTER TABLE "post_service"."comments" DROP CONSTRAINT IF EXISTS "users_fk", DROP COLUMN "author_id", ADD COLUMN "author_name" character varying(25) NOT NULL, ADD CONSTRAINT "users_fk" FOREIGN KEY ("author_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "likes" table
ALTER TABLE "post_service"."likes" DROP CONSTRAINT "likes_pkey", DROP CONSTRAINT IF EXISTS "users_fk", DROP COLUMN "user_id", ADD COLUMN "username" character varying(25) NOT NULL, ADD PRIMARY KEY ("username", "post_id"), ADD CONSTRAINT "users_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "posts" table
ALTER TABLE "post_service"."posts" DROP CONSTRAINT IF EXISTS "users_fk", DROP COLUMN "author_id", ADD COLUMN "author_name" character varying(25) NOT NULL, ADD CONSTRAINT "users_fk" FOREIGN KEY ("author_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "subscriptions" table
ALTER TABLE "user_service"."subscriptions" DROP CONSTRAINT "subscriptions_uq", DROP CONSTRAINT IF EXISTS "subscribee_fk", DROP CONSTRAINT IF EXISTS "subscriber_fk", DROP COLUMN "subscriber_id", DROP COLUMN "subscribee_id", ADD COLUMN "subscriber_name" character varying(25) NOT NULL, ADD COLUMN "subscribee_name" character varying(25) NOT NULL, ADD CONSTRAINT "subscriptions_uq" UNIQUE ("subscriber_name", "subscribee_name"), ADD CONSTRAINT "subscribee_fk" FOREIGN KEY ("subscribee_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "subscriber_fk" FOREIGN KEY ("subscriber_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "tokens" table
ALTER TABLE "user_service"."tokens" DROP CONSTRAINT "user_token_type_uq", DROP CONSTRAINT IF EXISTS "users_fk", DROP COLUMN "user_id", ADD COLUMN "username" character varying(25) NOT NULL, ADD CONSTRAINT "user_token_type_uq" UNIQUE ("username", "type"), ADD CONSTRAINT "users_fk" FOREIGN KEY ("username") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE;
