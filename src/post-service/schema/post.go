package schema

import (
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"time"
)

type Post struct {
	PostID        string    `db:"post_id"`
	Content       string    `db:"content"`
	CreatedAt     time.Time `db:"created_at"`
	AuthorName    string    `db:"author_name"`
	Longitude     *float32  `db:"longitude,omitempty"`
	Latitude      *float32  `db:"latitude,omitempty"`
	Accuracy      *int32    `db:"accuracy,omitempty"`
	PictureURL    *string   `db:"picture_url,omitempty"`
	PictureWidth  *int32    `db:"picture_width,omitempty"`
	PictureHeight *int32    `db:"picture_height,omitempty"`
	RepostPostID  *string   `db:"repost_post_id,omitempty"`
}

func (post *Post) ToProto(authorMap map[string]*userv1.User, repostMap map[string]*postv1.Post) *postv1.Post {
	proto := &postv1.Post{
		PostId:       post.PostID,
		CreationDate: post.CreatedAt.Format(time.RFC3339),
		Content:      post.Content,
		Author:       authorMap[post.AuthorName],
	}

	if post.Latitude != nil && post.Longitude != nil && post.Accuracy != nil {
		proto.Location = &postv1.Location{
			Latitude:  *post.Latitude,
			Longitude: *post.Longitude,
			Accuracy:  *post.Accuracy,
		}
	}

	if post.PictureURL != nil && post.PictureWidth != nil && post.PictureHeight != nil {
		proto.Picture = &imagev1.Picture{
			Url:    *post.PictureURL,
			Width:  *post.PictureWidth,
			Height: *post.PictureHeight,
		}
	}

	if post.RepostPostID != nil {
		if repost, exists := repostMap[*post.RepostPostID]; exists {
			proto.Repost = repost
		}
	}

	return proto
}
