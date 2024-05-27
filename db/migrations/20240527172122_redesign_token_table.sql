-- Create enum type "token_type"
CREATE TYPE "user_service"."token_type" AS ENUM ('activation', 'password_reset');
-- Create "tokens" table
CREATE TABLE "user_service"."tokens" ("token_id" uuid NOT NULL, "token" character varying(6) NOT NULL, "expires_at" timestamptz NOT NULL, "type" "user_service"."token_type" NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("token_id"), CONSTRAINT "user_token_type_uq" UNIQUE ("user_id", "type"), CONSTRAINT "users_fk" FOREIGN KEY ("user_id") REFERENCES "user_service"."users" ("user_id") ON UPDATE CASCADE ON DELETE CASCADE);
-- Drop "activation_tokens" table
DROP TABLE "user_service"."activation_tokens";
