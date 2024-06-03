-- Add new schema named "chat_service"
CREATE SCHEMA "chat_service";
-- Set comment to schema: "chat_service"
COMMENT ON SCHEMA "chat_service" IS 'Chat service schema';
-- Create "chats" table
CREATE TABLE "chat_service"."chats" ("chat_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "user1_name" character varying(25) NOT NULL, "user2_name" character varying(25) NOT NULL, PRIMARY KEY ("chat_id"), CONSTRAINT "users_uq" UNIQUE ("user1_name", "user2_name"), CONSTRAINT "user1_fk" FOREIGN KEY ("user1_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "user2_fk" FOREIGN KEY ("user2_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "chats_check" CHECK ((user1_name)::text < (user2_name)::text));
-- Create "messages" table
CREATE TABLE "chat_service"."messages" ("message_id" uuid NOT NULL, "content" character varying(256) NOT NULL, "created_at" timestamptz NOT NULL, "chat_id" uuid NOT NULL, "sender_name" character varying(25) NOT NULL, PRIMARY KEY ("message_id"), CONSTRAINT "chats_fk" FOREIGN KEY ("chat_id") REFERENCES "chat_service"."chats" ("chat_id") ON UPDATE CASCADE ON DELETE CASCADE, CONSTRAINT "users_fk" FOREIGN KEY ("sender_name") REFERENCES "user_service"."users" ("username") ON UPDATE CASCADE ON DELETE CASCADE);
