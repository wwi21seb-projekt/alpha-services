package handler

import (
	"context"
	"errors"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
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
	GetUserFeed(c *gin.Context)   // GET /users/:username/feed
}

type PostHandler struct {
	postService        pbPost.PostServiceClient
	feedService        pbPost.FeedServiceClient
	jwtManager         manager.JWTManager
	interactionService pbPost.InteractionServiceClient
}

func NewPostHandler(client pbPost.PostServiceClient, feedService pbPost.FeedServiceClient, jwtManager manager.JWTManager, interactionService pbPost.InteractionServiceClient) PostHdlr {
	return &PostHandler{
		postService:        client,
		feedService:        feedService,
		jwtManager:         jwtManager,
		interactionService: interactionService,
	}
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	// Parse request body to get post data
	createPostRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*schema.CreatePostRequest)
	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	req := &pbPost.CreatePostRequest{
		Content:  createPostRequest.Content,
		Location: helper.LocationToProto(&createPostRequest.Location),
	}

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
	publicFeedWanted := h.isPublicFeedWanted(c)

	lastPostID := c.Query("postId")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	var resp *pbPost.SearchPostsResponse

	if publicFeedWanted {
		resp, err = h.feedService.GetGlobalFeed(c, &pbPost.GetFeedRequest{
			LastPostId: lastPostID,
			Limit:      int32(limit),
		})
	} else {
		resp, err = h.feedService.GetPersonalFeed(c, &pbPost.GetFeedRequest{
			LastPostId: lastPostID,
			Limit:      int32(limit),
		})
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) isPublicFeedWanted(c *gin.Context) bool {
	authHeader := c.GetHeader("Authorization")
	feedType := c.Query("feedType")

	if authHeader == "" || feedType == "global" {
		return true
	}

	if !strings.HasPrefix(authHeader, "Bearer ") || len(authHeader) <= len("Bearer ") {
		err := errors.New("invalid authorization header")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}

	jwtToken := authHeader[len("Bearer "):]
	_, err := h.jwtManager.Verify(jwtToken)
	if err != nil {
		err := errors.New("invalid authorization header")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}

	return false
}

func (h *PostHandler) GetUserFeed(c *gin.Context) {
	user := c.Param("username")
	lastPostId := c.Query("postId")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	if user == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	// Save the username to context
	c.Set("username", user)

	resp, err := h.postService.ListPosts(c, &pbPost.ListPostsRequest{
		Author:      user,
		LikedBy:     user,
		CommentedBy: user,
		Pagination: &pbPost.PostPagination{
			LastPostId: lastPostId,
			Limit:      int32(limit),
		},
	})

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) CreateComment(c *gin.Context) {
	// to-do
}

func (h *PostHandler) GetComments(c *gin.Context) {
	// to-do
}

func (h *PostHandler) CreateLike(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := h.interactionService.LikePost(ctx, &pbPost.LikePostRequest{PostId: c.Param("postId")})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *PostHandler) DeleteLike(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := h.interactionService.UnlikePost(ctx, &pbPost.UnlikePostRequest{PostId: c.Param("postId")})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}
