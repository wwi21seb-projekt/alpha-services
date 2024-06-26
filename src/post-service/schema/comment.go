package schema

import "time"

type Comment struct {
	CommentID  string    `db:"comment_id"`
	Content    string    `db:"content"`
	CreatedAt  time.Time `db:"created_at"`
	AuthorName string    `db:"author_name"`
	PostID     string    `db:"post_id"`
}
