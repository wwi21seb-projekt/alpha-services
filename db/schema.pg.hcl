table "activation_tokens" {
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
    null = true
    type = timestamptz
  }
  column "user_id" {
    null = false
    type = uuid
  }
  primary_key {
    columns = [column.token_id]
  }
  foreign_key "users_fk" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.user_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  unique "users_tokens_uq" {
    columns = [column.user_id]
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
  column "author_id" {
    null = false
    type = uuid
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
    columns     = [column.author_id]
    ref_columns = [table.users.column.user_id]
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
  column "user_id" {
    null = false
    type = uuid
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
    columns = [column.user_id, column.post_id]
  }
  foreign_key "posts_fk" {
    columns     = [column.post_id]
    ref_columns = [table.posts.column.post_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "users_fk" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.user_id]
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
    null = true
    type = character_varying(256)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "author_id" {
    null = false
    type = uuid
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
  column "repost_post_id" {
    null = true
    type = uuid
  }
  primary_key {
    columns = [column.post_id]
  }
  foreign_key "users_fk" {
    columns     = [column.author_id]
    ref_columns = [table.users.column.user_id]
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
  column "subscriber_id" {
    null = false
    type = uuid
  }
  column "subscribee_id" {
    null = false
    type = uuid
  }
  primary_key {
    columns = [column.subscription_id]
  }
  foreign_key "subscribee_fk" {
    columns     = [column.subscribee_id]
    ref_columns = [table.users.column.user_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  foreign_key "subscriber_fk" {
    columns     = [column.subscriber_id]
    ref_columns = [table.users.column.user_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
  unique "subscriptions_uq" {
    columns = [column.subscriber_id, column.subscribee_id]
  }
}
table "users" {
  schema = schema.user_service
  column "user_id" {
    null = false
    type = uuid
  }
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
  column "profile_picture_url" {
    null = true
    type = character_varying(256)
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
    columns = [column.user_id]
  }
  unique "username_uq" {
    columns = [column.username]
  }
  unique "email_uq" {
    columns = [column.email]
  }
}
schema "user_service" {
  comment = "User service schema"
}
schema "post_service" {
  comment = "Post service schema"
}
