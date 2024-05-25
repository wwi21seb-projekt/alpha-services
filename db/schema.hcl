schema "user_service" {
  comment = "User service schema"
}

table "users" {
  schema = schema.user_service
  column "id" {
    type = int
    null = false
  }
  column "username" {
    type = varchar(50)
    null = false
  }
  column "email" {
    type = varchar(100)
    null = false
  }
  primary_key {
    columns = [
      column.id
    ]
  }
}

schema "post_service" {
  comment = "Post service schema"
}

table "posts" {
  schema = schema.post_service
  column "id" {
    type = int
    null = false
  }
  column "user_id" {
    type = int
    null = false

  }
  column "title" {
    type = varchar(100)
    null = false
  }
  column "content" {
    type = text
    null = false
  }
  primary_key {
    columns = [
      column.id
    ]
  }
  foreign_key "fk_user_id" {
    ref_columns = [table.users.column.id]
    columns = [column.user_id]
  }
}

table "likes" {
  schema = schema.post_service
  column "id" {
    type = int
    null = false
  }
  column "user_id" {
    type = int
    null = false
  }
  column "post_id" {
    type = int
    null = false
  }
  primary_key {
      columns = [column.id]
  }
  foreign_key "fk_user_id" {
    ref_columns = [table.users.column.id]
    columns = [column.user_id]
  }
  foreign_key "fk_post_id" {
    ref_columns = [table.posts.column.id]
    columns = [column.post_id]
  }
}
