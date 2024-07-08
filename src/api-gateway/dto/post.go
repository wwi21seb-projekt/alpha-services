package dto

import (
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
)

// ======================================== //
// =========== Shared DTOs ================ //
// ======================================== //

type Location struct {
	Latitude  float32 `json:"latitude" validate:"latitude,required_with=Longitude"`
	Longitude float32 `json:"longitude" validate:"longitude,required_with=Latitude"`
	Accuracy  int32   `json:"accuracy" validate:"gte=0"`
}

// ======================================== //
// =========== Request DTOs =============== //
// ======================================== //

type CreatePostRequest struct {
	Content        string    `json:"content" validate:"max=256,post_validation,required_without=Picture"`
	Picture        *string   `json:"picture" validate:"omitempty"`
	RepostedPostID *string   `json:"repostedPostId" validate:"omitempty,uuid4"`
	Location       *Location `json:"location" validate:"omitempty"`
}

// ======================================== //
// =========== Response DTOs ============== //
// ======================================== //

type Post struct {
	PostID       string    `json:"postId"`
	Author       User      `json:"author"`
	CreationDate string    `json:"creationDate"`
	Content      string    `json:"content"`
	Picture      *Picture  `json:"picture,omitempty"`
	Location     *Location `json:"location,omitempty"`
	Likes        uint32    `json:"likes"`
	Liked        bool      `json:"liked"`
	Comments     uint32    `json:"comments"`
	Repost       *Repost   `json:"repost"`
}

type Repost struct {
	//	PostID       string    `json:"repostedPostId"` // 2024-02-01T16:18:48+01:00
	Author       User      `json:"author"`
	CreationDate string    `json:"creationDate"`
	Content      string    `json:"content"`
	Picture      *Picture  `json:"picture,omitempty"`
	Location     *Location `json:"location,omitempty"`
}

type GetUserFeedResponse struct {
	Posts      []Post             `json:"records"`
	Pagination PaginationResponse `json:"pagination"`
}

type GetFeedResponse struct {
	Posts      []Post                 `json:"records"`
	Pagination PostPaginationResponse `json:"pagination"`
}

type SearchPostsResponse struct {
	Posts      []Post                 `json:"records"`
	Pagination PostPaginationResponse `json:"pagination"`
}

type PostPaginationResponse struct {
	LastPostID string `json:"lastPostId"`
	Limit      int32  `json:"limit"`
	Records    int32  `json:"records"`
}

// ======================================== //
// ========== Helper Functions ============ //
// ======================================== //

func TransformProtoLocationToDTO(location *postv1.Location) *Location {
	if location == nil {
		return nil
	}
	return &Location{
		Latitude:  location.GetLatitude(),
		Longitude: location.GetLongitude(),
		Accuracy:  location.GetAccuracy(),
	}
}

func TransformProtoPostToDTO(post *postv1.Post) *Post {
	if post == nil {
		return nil
	}

	return &Post{
		PostID:       post.GetPostId(),
		Author:       TransformProtoUserToDTO(post.GetAuthor()),
		CreationDate: post.GetCreationDate(),
		Content:      post.GetContent(),
		Picture:      TransformProtoPicToDTO(post.GetPicture()),
		Location:     TransformProtoLocationToDTO(post.GetLocation()),
		Likes:        post.GetLikes(),
		Liked:        post.GetLiked(),
		Comments:     post.GetComments(),
		Repost:       TransformProtoRepostToDTO(post.GetRepost()),
	}
}

func TransformProtoRepostToDTO(repost *postv1.Post) *Repost {
	if repost == nil {
		return nil
	}
	repostDTO := &Repost{
		//	PostID:       repost.GetPostId(),
		Author:       TransformProtoUserToDTO(repost.GetAuthor()),
		CreationDate: repost.GetCreationDate(),
		Content:      repost.GetContent(),
		Picture:      nil,
		Location:     nil,
	}

	if repost.GetAuthor() != nil {
		repostDTO.Author = TransformProtoUserToDTO(repost.GetAuthor())
	}

	if repost.GetPicture() != nil {
		repostDTO.Picture = TransformProtoPicToDTO(repost.GetPicture())
	}

	if repost.GetLocation() != nil {
		TransformProtoLocationToDTO(repost.GetLocation())
	}

	return repostDTO
}
