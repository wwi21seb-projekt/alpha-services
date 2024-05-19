package handler

import (
	"context"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/api-gateway/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-services/post-service/proto"
	"go-micro.dev/v4/logger"
	"go-micro.dev/v4/metadata"
)

type PostHandler struct {
	PostService pbPost.PostService
}

func NewPostHandler(postService pbPost.PostService) *PostHandler {
	return &PostHandler{
		PostService: postService,
	}
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	// Get JWT claims from context
	claims := c.MustGet("claims").(jwt.MapClaims)
	// Parse request body to get post data
	createPostRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*schema.CreatePostRequest)

	req := &pbPost.CreatePostRequest{
		Content:  createPostRequest.Content,
		Location: &createPostRequest.Location,
	}

	// Create a context with the userId from the JWT claims
	ctx := metadata.NewContext(context.Background(), map[string]string{
		"userId": claims["sub"].(string),
	})

	// Call CreatePost method on postService
	rsp, err := h.PostService.CreatePost(ctx, req)
	if err != nil {
		logger.Error(err)
		c.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Return the response
	c.JSON(200, rsp)
}
