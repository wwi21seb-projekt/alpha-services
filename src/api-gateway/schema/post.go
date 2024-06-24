package schema

import serveralpha "github.com/wwi21seb-projekt/alpha-shared/proto/post"

// ------------------------------------------
// ------------ Request DTOs ----------------
// ------------------------------------------

type Location struct {
	Latitude  float32 `json:"latitude" validate:"latitude,required_with=Longitude"`
	Longitude float32 `json:"longitude" validate:"longitude,required_with=Latitude"`
	Accuracy  int32   `json:"accuracy" validate:"gte=0"`
}

// CreatePostRequest is a struct that represents a create post request
// Content is required and must be less than 256 characters, as well as written in UTF-8
// Location is optional and must be a valid location if provided
type CreatePostRequest struct {
	RepostedPostID *string   `json:"repostedPostId,omitempty" validate:"omitempty,uuid4"`
	Content        string    `json:"content" validate:"max=256,post_validation,required_without=Picture"`
	Picture        *string   `json:"picture" validate:"omitempty"`
	Location       *Location `json:"location" validate:"omitempty"`
}

type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,max=128"`
}

type FeedResponse struct {
	Posts      []serveralpha.Post `json:"records"`
	Pagination PostPagination     `json:"pagination"`
}

type PostPagination struct {
	LastPostID string `json:"lastPostId"`
	Limit      int32  `json:"limit"`
	Records    int32  `json:"records"`
}
