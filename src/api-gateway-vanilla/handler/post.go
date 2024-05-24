package handler

import (
	"context"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
)

type PostHandler struct {
	postService pbPost.PostServiceClient
}

func NewPostHandler(client pbPost.PostServiceClient) *PostHandler {
	return &PostHandler{
		postService: client,
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
	ctx := context.WithValue(c, "userId", claims["sub"].(string))

	// Call CreatePost method on postService
	rsp, err := h.postService.CreatePost(ctx, req)
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Return the response
	c.JSON(200, rsp)
}

func (h *PostHandler) GetFeed(c *gin.Context) {
	// Determine if user is authenticated or not
	// to-do

	// Call GetFeed method on postService
	rsp, err := h.postService.GetFeed(c, &pbPost.GetFeedRequest{})
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Return the response
	c.JSON(200, rsp)
}
