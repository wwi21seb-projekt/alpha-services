package schema

import pbPost "github.com/wwi21seb-projekt/alpha-services/post-service/proto"

// ------------------------------------------
// ------------ Request DTOs ----------------
// ------------------------------------------

// CreatePostRequest is a struct that represents a create post request
// Content is required and must be less than 256 characters, as well as written in UTF-8
// Location is optional and must be a valid location if provided
type CreatePostRequest struct {
	Content  string          `json:"content" validate:"required,max=256,post_validation"`
	Location pbPost.Location `json:"location" validate:"location_validation"`
}
