package dto

import postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"

// ======================================== //
// =========== Shared DTOs ================ //
// ======================================== //

type Comment struct {
	CommentID    string `json:"commentId"`
	Content      string `json:"content"`
	Author       User   `json:"author"`
	CreationDate string `json:"creationDate"`
}

// ======================================== //
// ============ Request DTOs ============== //
// ======================================== //

type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,max=256"`
}

// ======================================== //
// ============ Response DTOs ============= //
// ======================================== //

type ListCommentsResponse struct {
	Comments   []Comment          `json:"records"`
	Pagination PaginationResponse `json:"pagination"`
}

// ======================================== //
// ========== Helper Functions ============ //
// ======================================== //

func TransformProtoCommentToDTO(comment *postv1.CreateCommentResponse) *Comment {
	if comment == nil {
		return nil
	}
	return &Comment{
		CommentID:    comment.GetCommentId(),
		Content:      comment.GetContent(),
		Author:       TransformProtoUserToDTO(comment.GetAuthor()),
		CreationDate: comment.GetCreationDate(),
	}
}
