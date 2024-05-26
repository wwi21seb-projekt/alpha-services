package schema

// ------------------------------------------
// ------------ Request DTOs ----------------
// ------------------------------------------

type Location struct {
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
	Accuracy  int32   `json:"accuracy"`
}

// CreatePostRequest is a struct that represents a create post request
// Content is required and must be less than 256 characters, as well as written in UTF-8
// Location is optional and must be a valid location if provided
type CreatePostRequest struct {
	Content  string   `json:"content" validate:"required,max=256,post_validation"`
	Location Location `json:"location,omitempty" validate:"location_validation"`
}

type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,max=128"`
}
