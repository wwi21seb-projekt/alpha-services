package handler

import (
	"context"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helpers"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	"net/http"
)

type PostHdlr interface {
	CreatePost(c *gin.Context)    // POST /posts
	QueryPosts(c *gin.Context)    // GET /posts
	DeletePost(c *gin.Context)    // DELETE /posts/:postId
	GetFeed(c *gin.Context)       // GET /feed
	CreateComment(c *gin.Context) // POST /posts/:postId/comments
	GetComments(c *gin.Context)   // GET /posts/:postId/comments
	CreateLike(c *gin.Context)    // POST /posts/:postId/likes
	DeleteLike(c *gin.Context)    // DELETE /posts/:postId/likes
}

type PostHandler struct {
	postService pbPost.PostServiceClient
}

func NewPostHandler(client pbPost.PostServiceClient) PostHdlr {
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
		Location: helpers.LocationToProto(&createPostRequest.Location),
	}

	// Create a context with the userId from the JWT claims
	ctx := context.WithValue(context.Background(), "userId", claims["sub"].(string))

	// Call CreatePost method on postService
	rsp, err := h.postService.CreatePost(ctx, req)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the response
	c.JSON(http.StatusOK, rsp)
}

func (h *PostHandler) QueryPosts(c *gin.Context) {
	// to-do
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	// to-do
}

func (h *PostHandler) GetFeed(c *gin.Context) {
	// to-do
}

func (h *PostHandler) CreateComment(c *gin.Context) {
	// to-do
}

func (h *PostHandler) GetComments(c *gin.Context) {
	// to-do
}

func (h *PostHandler) CreateLike(c *gin.Context) {
	// to-do
}

func (h *PostHandler) DeleteLike(c *gin.Context) {
	// to-do
}
