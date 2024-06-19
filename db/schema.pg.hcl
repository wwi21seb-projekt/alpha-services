enum "token_type" {
  schema = schema.user_service
  values = ["activation", "password_reset"]
}
enum "notification_type" {
  schema = schema.notification_service
  values = ["follow", "repost"]
}
enum "subscription_type" {
  schema = schema.notification_service
  values = ["expo", "web"]
}

// You need to be logged in to the Atla CLI to run diff.
extension "fuzzystrmatch" {
  schema = schema.user_service
  comment = "Fuzzy string matching functions, especially levenshtein" 
}

table "tokens" {
  schema = schema.user_service
  column "token_id" {
    null = false
    type = uuid
  }
  column "token" {
    null = false
    type = character_varying(6)
  }
  column "expires_at" {
    null = false
    type = timestamptz
  }
  column "type" {
    null = false
    type = enum.token_type
  }
  column "username" {
    null = false
    type = character_varying(25)
  }
  primary_key {
    columns = [column.token_id]
  }
  foreign_key "users_fk" {
    columns     = [column.username]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  unique "user_token_type_uq" {
    columns = [column.username, column.type]
  }
}

table "comments" {
  schema = schema.post_service
  column "comment_id" {
    null = false
    type = uuid
  }
  column "content" {
    null = false
    type = character_varying(128)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "author_name" {
    null = false
    type = character_varying(25)
  }
  column "post_id" {
    null = false
    type = uuid
  }
  primary_key {
    columns = [column.comment_id]
  }
  foreign_key "posts_fk" {
    columns     = [column.post_id]
    ref_columns = [table.posts.column.post_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "users_fk" {
    columns     = [column.author_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}

table "hashtags" {
  schema = schema.post_service
  column "hashtag_id" {
    null = false
    type = uuid
  }
  column "content" {
    null = false
    type = character_varying(32)
  }
  primary_key {
    columns = [column.hashtag_id]
  }
  unique "hashtags_uq" {
    columns = [column.content]
  }
}

table "likes" {
  schema = schema.post_service
  column "username" {
    null = false
    type = character_varying(25)
  }
  column "post_id" {
    null = false
    type = uuid
  }
  column "liked_at" {
    null = false
    type = timestamptz
  }
  primary_key {
    columns = [column.username, column.post_id]
  }
  foreign_key "posts_fk" {
    columns     = [column.post_id]
    ref_columns = [table.posts.column.post_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "users_fk" {
    columns     = [column.username]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}

table "many_posts_has_many_hashtags" {
  schema = schema.post_service
  column "post_id_posts" {
    null = false
    type = uuid
  }
  column "hashtag_id_hashtags" {
    null = false
    type = uuid
  }
  primary_key {
    columns = [column.post_id_posts, column.hashtag_id_hashtags]
  }
  foreign_key "hashtags_fk" {
    columns     = [column.hashtag_id_hashtags]
    ref_columns = [table.hashtags.column.hashtag_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "posts_fk" {
    columns     = [column.post_id_posts]
    ref_columns = [table.posts.column.post_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}

table "posts" {
  schema = schema.post_service
  column "post_id" {
    null = false
    type = uuid
  }
  column "content" {
    null = false
    type = character_varying(256)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "author_name" {
    null = false
    type = character_varying(25)
  }
  column "longitude" {
    null = true
    type = double_precision
  }
  column "latitude" {
    null = true
    type = double_precision
  }
  column "accuracy" {
    null = true
    type = integer
  }
  column "picture_url" {
    null = false
    type = character_varying(64)
  }
  column "picture_width" {
    null = false
    type = integer
  }
  column "picture_height" {
    null = false
    type = integer
  }
  column "repost_post_id" {
    null = true
    type = uuid
  }
  primary_key {
    columns = [column.post_id]
  }
  foreign_key "users_fk" {
    columns     = [column.author_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}

table "subscriptions" {
  schema = schema.user_service
  column "subscription_id" {
    null = false
    type = uuid
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "subscriber_name" {
    null = false
    type = character_varying(25)
  }
  column "subscribee_name" {
    null = false
    type = character_varying(25)
  }
  primary_key {
    columns = [column.subscription_id]
  }
  foreign_key "subscribee_fk" {
    columns     = [column.subscribee_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "subscriber_fk" {
    columns     = [column.subscriber_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  unique "subscriptions_uq" {
    columns = [column.subscriber_name, column.subscribee_name]
  }
}

table "users" {
  schema = schema.user_service
  column "username" {
    null = false
    type = character_varying(25)
  }
  column "nickname" {
    null = false
    type = character_varying(20)
  }
  column "email" {
    null = false
    type = character_varying(128)
  }
  column "password" {
    null = false
    type = character(60)
  }
  column "status" {
    null = true
    type = character_varying(256)
  }
  column "picture_url" {
    null = false
    type = character_varying(64)
  }
  column "picture_width" {
    null = false
    type = integer
  }
  column "picture_height" {
    null = false
    type = integer
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "activated_at" {
    null = true
    type = timestamptz
  }
  column "expires_at" {
    null = true
    type = timestamptz
  }
  primary_key {
    columns = [column.username]
  }
  unique "email_uq" {
    columns = [column.email]
  }
}

table "chats" {
  schema = schema.chat_service
  column "chat_id" {
    null = false
    type = uuid
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "user1_name" {
    null = false
    type = character_varying(25)
  }
  column "user2_name" {
    null = false
    type = character_varying(25)
  }
  primary_key {
    columns = [column.chat_id]
  }
  foreign_key "user1_fk" {
    columns     = [column.user1_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "user2_fk" {
    columns     = [column.user2_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  check  {
    expr = "user1_name < user2_name"
  }
  // Ensure that two users can only have one chat
  unique "users_uq" {
    columns = [column.user1_name, column.user2_name]
  }
}

table "messages" {
  schema = schema.chat_service
  column "message_id" {
    null = false
    type = uuid
  }
  column "content" {
    null = false
    type = character_varying(256)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "chat_id" {
    null = false
    type = uuid
  }
  column "sender_name" {
    null = false
    type = character_varying(25)
  }
  primary_key {
    columns = [column.message_id]
  }
  foreign_key "chats_fk" {
    columns     = [column.chat_id]
    ref_columns = [table.chats.column.chat_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "users_fk" {
    columns     = [column.sender_name]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}


table "push_subscriptions" {
  schema = schema.notification_service
  column "subscription_id" {
    null = false
    type = uuid
  }
  column "username" {
    null = false
    type = character_varying(25)
  }
  column "type" {
    null = false
    type = enum.subscription_type
  }
  column "token" {
    null = true
    type = character_varying(64)
  }
  column "endpoint" {
    null = true
    type = character_varying(256)
  }
  column "expiration_time" {
    null = true
    type = timestamptz
  }
  column "p256dh" {
    null = true
    type = character_varying(256)
  }
  column "auth" {
    null = true
    type = character_varying(256)
  }
  primary_key {
    columns = [column.subscription_id]
  }
  foreign_key "username_fk" {
  columns     = [column.username]
  ref_columns = [table.users.column.username]
  on_update   = CASCADE
  on_delete   = CASCADE
  }
}
table "notifications"{
  schema = schema.notification_service
  column "notification_id" {
    null = false
    type = uuid
  }
  column "recipient_username" {
    null = false
    type = character_varying(25)
  }
  column "notification_type" {
    null = false
    type = enum.notification_type
  }
  column "sender_username" {
    null = false
    type = character_varying(25)
  }
  column "timestamp" {
    null = false
    type = timestamptz
  }
  primary_key {
    columns = [column.notification_id]
  }
  foreign_key "recipient_fk" {
    columns     = [column.recipient_username]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "sender_fk" {
    columns     = [column.sender_username]
    ref_columns = [table.users.column.username]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}
schema "user_service" {
  comment = "User service schema"
}
schema "post_service" {
  comment = "Post service schema"
}
schema "chat_service" {
  comment = "Chat service schema"
}
schema "notification_service" {
  comment = "Notification service schema"
}
