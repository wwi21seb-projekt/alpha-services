package handler

import (
	"context"
	"errors"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	GetUserFeed(c *gin.Context)   // GET /users/:username/feed
	GetFeed(c *gin.Context)       // GET /feed
	CreateComment(c *gin.Context) // POST /posts/:postId/comments
	GetComments(c *gin.Context)   // GET /posts/:postId/comments
	CreateLike(c *gin.Context)    // POST /posts/:postId/likes
	DeleteLike(c *gin.Context)    // DELETE /posts/:postId/likes
}

type PostHandler struct {
	logger      *zap.SugaredLogger
	tracer      trace.Tracer
	postService pbPost.PostServiceClient
	jwtManager  manager.JWTManager
}

func NewPostHandler(logger *zap.SugaredLogger, client pbPost.PostServiceClient, jwtManager manager.JWTManager) PostHdlr {
	return &PostHandler{
		logger:      logger,
		tracer:      otel.GetTracerProvider().Tracer("post-handler"),
		postService: client,
		jwtManager:  jwtManager,
	}
}

func (ph *PostHandler) CreatePost(c *gin.Context) {
	createPostRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*schema.CreatePostRequest)

	req := &pbPost.CreatePostRequest{
		Content:        createPostRequest.Content,
		Location:       helper.LocationToProto(&createPostRequest.Location),
		Picture:        &createPostRequest.Picture,
		RepostedPostId: &createPostRequest.RepostedPostID,
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	rsp, err := ph.postService.CreatePost(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.CreatePost: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.JSON(http.StatusCreated, rsp)
}

func (ph *PostHandler) GetUserFeed(c *gin.Context) {
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

	resp, err := ph.postService.ListPosts(c, &pbPost.ListPostsRequest{
		FeedType: pbPost.FeedType_FEED_TYPE_USER,
		Username: &user,
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

func (ph *PostHandler) QueryPosts(c *gin.Context) {
	// to-do
}

func (ph *PostHandler) DeletePost(c *gin.Context) {
	// to-do
}

func (ph *PostHandler) GetFeed(c *gin.Context) {
	publicFeedWanted := ph.isPublicFeedWanted(c)

	lastPostID := c.Query("postId")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	var resp *pbPost.ListPostsResponse

	if publicFeedWanted {
		resp, err = ph.postService.ListPosts(c, &pbPost.ListPostsRequest{
			FeedType: pbPost.FeedType_FEED_TYPE_GLOBAL,
			Pagination: &pbPost.PostPagination{
				LastPostId: lastPostID,
				Limit:      int32(limit),
			},
		})
	} else {
		ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

		resp, err = ph.postService.ListPosts(ctx, &pbPost.ListPostsRequest{
			FeedType: pbPost.FeedType_FEED_TYPE_PERSONAL,
			Pagination: &pbPost.PostPagination{
				LastPostId: lastPostID,
				Limit:      int32(limit),
			},
		})
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (ph *PostHandler) isPublicFeedWanted(c *gin.Context) bool {
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
	_, err := ph.jwtManager.Verify(jwtToken)
	if err != nil {
		err := errors.New("invalid authorization header")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}

	claimsfunc := middleware.SetClaimsMiddleware(ph.logger, ph.jwtManager)
	claimsfunc(c)

	return false
}

func (ph *PostHandler) CreateComment(c *gin.Context) {
	// to-do
}

func (ph *PostHandler) GetComments(c *gin.Context) {
	// to-do
}

func (ph *PostHandler) CreateLike(c *gin.Context) {
	// to-do
}

func (ph *PostHandler) DeleteLike(c *gin.Context) {
	// to-do
}
