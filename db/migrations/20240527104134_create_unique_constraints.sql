-- Modify "activation_tokens" table
ALTER TABLE "user_service"."activation_tokens" ADD CONSTRAINT "users_tokens_uq" UNIQUE ("user_id");
-- Modify "users" table
ALTER TABLE "user_service"."users" ADD CONSTRAINT "email_uq" UNIQUE ("email"), ADD CONSTRAINT "username_uq" UNIQUE ("username");
