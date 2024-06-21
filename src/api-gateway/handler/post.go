package handler

import (
	"context"
	"errors"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
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
	logger             *zap.SugaredLogger
	tracer             trace.Tracer
	jwtManager         manager.JWTManager
	postService        pbPost.PostServiceClient
	interactionService pbPost.InteractionServiceClient
	middleware         middleware.Middleware
}

func NewPostHandler(logger *zap.SugaredLogger, client pbPost.PostServiceClient, jwtManager manager.JWTManager, interactionClient pbPost.InteractionServiceClient, middleware middleware.Middleware) PostHdlr {
	return &PostHandler{
		logger:             logger,
		tracer:             otel.GetTracerProvider().Tracer("post-handler"),
		postService:        client,
		jwtManager:         jwtManager,
		interactionService: interactionClient,
		middleware:         middleware,
	}
}

func (ph *PostHandler) CreatePost(c *gin.Context) {
	createPostRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*schema.CreatePostRequest)

	req := &pbPost.CreatePostRequest{
		Content:        createPostRequest.Content,
		Location:       helper.LocationToProto(createPostRequest.Location),
		Picture:        createPostRequest.Picture,
		RepostedPostId: createPostRequest.RepostedPostID,
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

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	resp, err := ph.postService.ListPosts(ctx, &pbPost.ListPostsRequest{
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
	query := c.Query("q")
	lastPostId := c.Query("postId")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	resp, err := ph.postService.ListPosts(ctx, &pbPost.ListPostsRequest{
		FeedType: pbPost.FeedType_FEED_TYPE_GLOBAL,
		Hashtag:  &query,
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

func (ph *PostHandler) DeletePost(c *gin.Context) {
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	req := &pbPost.GetPostRequest{
		PostId: postID,
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := ph.postService.DeletePost(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.DeletePost: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.Status(http.StatusNoContent)
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

	claimsfunc := ph.middleware.SetClaimsMiddleware()
	claimsfunc(c)

	return false
}

func (ph *PostHandler) CreateComment(c *gin.Context) {
	createCommentRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*schema.CreateCommentRequest)
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	req := &pbPost.CreateCommentRequest{
		PostId:  postID,
		Content: createCommentRequest.Content,
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	rsp, err := ph.interactionService.CreateComment(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.CreateComment: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.JSON(http.StatusCreated, rsp)
}

func (ph *PostHandler) GetComments(c *gin.Context) {
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		offset = 0
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	req := &pbPost.ListCommentsRequest{
		PostId: postID,
		Pagination: &pbCommon.PaginationRequest{
			Offset: int32(offset),
			Limit:  int32(limit),
		},
	}

	rsp, err := ph.interactionService.ListComments(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.ListComments: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.JSON(http.StatusOK, rsp)
}

func (ph *PostHandler) CreateLike(c *gin.Context) {
	// Get post id from path
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	req := &pbPost.LikePostRequest{
		PostId: postID,
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := ph.interactionService.LikePost(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.LikePost: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.Status(http.StatusCreated)
}

func (ph *PostHandler) DeleteLike(c *gin.Context) {
	// Get post id from path
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	req := &pbPost.UnlikePostRequest{
		PostId: postID,
	}

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := ph.interactionService.UnlikePost(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.NotLiked
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		}

		ph.logger.Errorf("error in upstream call uh.postService.UnlikePost: %v", err)
		c.JSON(returnErr.HttpStatus, &schema.ErrorDTO{Error: returnErr})
		return
	}

	c.Status(http.StatusNoContent)
}
