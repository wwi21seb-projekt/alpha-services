package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	postService        postv1.PostServiceClient
	interactionService postv1.InteractionServiceClient
	middleware         middleware.Middleware
}

func NewPostHandler(logger *zap.SugaredLogger, client postv1.PostServiceClient, jwtManager manager.JWTManager, interactionClient postv1.InteractionServiceClient, middleware middleware.Middleware) PostHdlr {
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
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)
	createPostRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*dto.CreatePostRequest)

	req := &postv1.CreatePostRequest{
		Content:        createPostRequest.Content,
		Picture:        createPostRequest.Picture,
		RepostedPostId: createPostRequest.RepostedPostID,
	}

	if createPostRequest.Location != nil {
		loc := createPostRequest.Location
		req.Location = &postv1.Location{
			Latitude:  loc.Latitude,
			Longitude: loc.Longitude,
			Accuracy:  loc.Accuracy,
		}
	}

	rsp, err := ph.postService.CreatePost(ctx, req)
	if err != nil {
		var returnErr *goerrors.CustomError
		switch status.Convert(err).Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		case codes.PermissionDenied:
			returnErr = goerrors.UserNotActivated
		default:
			returnErr = goerrors.InternalServerError
		}

		ph.logger.Errorf("error in upstream call uh.postService.CreatePost: %v", err)
		c.JSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
		return
	}

	postDTO := dto.Post{
		PostID: rsp.GetPostId(),
		Author: dto.User{
			Username: rsp.GetAuthor().Username,
			Nickname: rsp.GetAuthor().Nickname,
			Picture:  nil,
		},
		CreationDate: rsp.GetCreationDate(),
		Content:      rsp.GetContent(),
		Picture:      nil,
		Location:     nil,
		Likes:        rsp.GetLikes(),
		Liked:        rsp.GetLiked(),
		Repost:       nil,
	}

	if rsp.GetAuthor().GetPicture() != nil {
		postDTO.Author.Picture = &dto.Picture{
			URL:    rsp.GetAuthor().GetPicture().GetUrl(),
			Width:  rsp.GetAuthor().GetPicture().GetWidth(),
			Height: rsp.GetAuthor().GetPicture().GetHeight(),
		}
	}

	if rsp.GetLocation() != nil {
		postDTO.Location = &dto.Location{
			Latitude:  rsp.GetLocation().GetLatitude(),
			Longitude: rsp.GetLocation().GetLongitude(),
			Accuracy:  rsp.GetLocation().GetAccuracy(),
		}
	}

	if rsp.GetPicture() != nil {
		postDTO.Picture = &dto.Picture{
			URL:    rsp.GetPicture().GetUrl(),
			Width:  rsp.GetPicture().GetWidth(),
			Height: rsp.GetPicture().GetHeight(),
		}
	}

	if rsp.GetRepost() != nil {
		repost := rsp.GetRepost()
		postDTO.Repost = &dto.Repost{
			Author: dto.User{
				Username: repost.GetAuthor().GetUsername(),
				Nickname: repost.GetAuthor().GetNickname(),
				Picture:  nil,
			},
			CreationDate: repost.GetCreationDate(),
			Content:      repost.GetContent(),
			Picture:      nil,
			Location:     nil,
		}

		if repost.GetAuthor().GetPicture() != nil {
			postDTO.Repost.Author.Picture = &dto.Picture{
				URL:    repost.GetAuthor().GetPicture().GetUrl(),
				Width:  repost.GetAuthor().GetPicture().GetWidth(),
				Height: repost.GetAuthor().GetPicture().GetHeight(),
			}
		}

		if repost.GetLocation() != nil {
			postDTO.Repost.Location = &dto.Location{
				Latitude:  repost.GetLocation().GetLatitude(),
				Longitude: repost.GetLocation().GetLongitude(),
				Accuracy:  repost.GetLocation().GetAccuracy(),
			}
		}

		if repost.GetPicture() != nil {
			postDTO.Repost.Picture = &dto.Picture{
				URL:    repost.GetPicture().GetUrl(),
				Width:  repost.GetPicture().GetWidth(),
				Height: repost.GetPicture().GetHeight(),
			}

		}
	}

	c.JSON(http.StatusCreated, postDTO)
}

func (ph *PostHandler) GetUserFeed(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	user := c.Param("username")
	offset := c.Query("offset")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 10
	}

	if user == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	resp, err := ph.postService.ListPosts(ctx, &postv1.ListPostsRequest{
		FeedType: postv1.FeedType_FEED_TYPE_USER,
		Username: &user,
		Pagination: &commonv1.PaginationRequest{
			PageToken: offset,
			PageSize:  int32(limit),
		},
	})

	if err != nil {
		if status.Code(err) == codes.NotFound {
			c.JSON(http.StatusNotFound, goerrors.UserNotFound)
			return
		}

		c.AbortWithStatusJSON(http.StatusInternalServerError, goerrors.InternalServerError)
		return
	}

	offsetInt, err := strconv.Atoi(offset)
	if err != nil {
		ph.logger.Warnw("error converting offset to int", "error", err)
		offsetInt = 0
	}

	posts := transformListPostsResponse(resp)

	feedResponse := &dto.GetUserFeedResponse{
		Posts: posts,
		Pagination: dto.PaginationResponse{
			Offset:  int32(offsetInt + limit),
			Limit:   int32(limit),
			Records: resp.GetPagination().GetTotalSize(),
		},
	}

	c.JSON(http.StatusOK, feedResponse)
}

func (ph *PostHandler) QueryPosts(c *gin.Context) {
	query := c.Query("q")
	lastPostId := c.Query("postId")

	var limit int32
	l, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		l = 10
	}
	limit = int32(l)

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	req := &postv1.ListPostsRequest{
		FeedType: postv1.FeedType_FEED_TYPE_GLOBAL,
		Hashtag:  &query,
		Pagination: &commonv1.PaginationRequest{
			PageToken: lastPostId,
			PageSize:  limit,
		},
	}

	resp, err := ph.postService.ListPosts(ctx, req)
	if err != nil {
		ph.logger.Errorw("error in upstream call uh.postService.ListPosts", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, goerrors.InternalServerError)
		return
	}

	posts := transformListPostsResponse(resp)

	feedResponse := &dto.GetFeedResponse{
		Posts: posts,
		Pagination: dto.PostPaginationResponse{
			LastPostID: resp.GetPagination().GetNextPageToken(),
			Limit:      limit,
			Records:    resp.GetPagination().GetTotalSize(),
		},
	}

	c.JSON(http.StatusOK, feedResponse)
}

func transformListPostsResponse(resp *postv1.ListPostsResponse) []dto.Post {
	posts := make([]dto.Post, 0, len(resp.GetPosts()))
	zap.L().Info("transforming posts")
	for _, post := range resp.GetPosts() {
		postDTO := dto.TransformProtoPostToDTO(post)
		if postDTO != nil {
			zap.L().Info("appending post")
			posts = append(posts, *postDTO)
		}
	}
	return posts
}

func (ph *PostHandler) DeletePost(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	postId := c.Param("postId")
	if postId == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	_, err := ph.postService.DeletePost(ctx, &postv1.DeletePostRequest{PostId: postId})
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		switch rpcStatus.Code() {
		case codes.NotFound:
			returnErr = goerrors.PostNotFound
		case codes.InvalidArgument:
			returnErr = goerrors.BadRequest
		case codes.PermissionDenied:
			returnErr = goerrors.DeletePostForbidden
		}

		ph.logger.Errorf("error in upstream call uh.postService.DeletePost: %v", err)
		c.AbortWithStatusJSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
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

	feedType := postv1.FeedType_FEED_TYPE_GLOBAL

	var ctx context.Context
	ctx = c
	if !publicFeedWanted {
		feedType = postv1.FeedType_FEED_TYPE_PERSONAL
		ctx = c.MustGet(middleware.GRPCMetadataKey).(context.Context)
	}

	resp, err := ph.postService.ListPosts(ctx, &postv1.ListPostsRequest{
		FeedType: feedType,
		Pagination: &commonv1.PaginationRequest{
			PageToken: lastPostID,
			PageSize:  int32(limit),
		},
	})

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": goerrors.InternalServerError})
		return
	}

	posts := transformListPostsResponse(resp)

	feedResponse := &dto.GetFeedResponse{
		Posts: posts,
		Pagination: dto.PostPaginationResponse{
			LastPostID: resp.GetPagination().GetNextPageToken(),
			Limit:      int32(limit),
			Records:    resp.GetPagination().GetTotalSize(),
		},
	}

	c.JSON(http.StatusOK, feedResponse)
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
	createCommentRequest := c.Value(middleware.SanitizedPayloadKey.String()).(*dto.CreateCommentRequest)
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, dto.ErrorDTO{Error: goerrors.PostNotFound})
	}

	req := &postv1.CreateCommentRequest{
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
		c.JSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
		return
	}

	c.JSON(http.StatusCreated, *dto.TransformProtoCommentToDTO(rsp))
}

func (ph *PostHandler) GetComments(c *gin.Context) {
	postID := c.Param("postId")
	if _, err := uuid.Parse(postID); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.PostNotFound)
		return
	}

	offset, limit := helper.ExtractPaginationFromContext(c)
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	req := &postv1.ListCommentsRequest{
		PostId: postID,
		Pagination: &commonv1.PaginationRequest{
			PageToken: strconv.FormatInt(offset, 10),
			PageSize:  limit,
		},
	}

	rsp, err := ph.interactionService.ListComments(ctx, req)
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		if rpcStatus.Code() == codes.NotFound {
			returnErr = goerrors.PostNotFound
		}

		ph.logger.Errorf("error in upstream call uh.postService.ListComments: %v", err)
		c.JSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
		return
	}

	comments := make([]dto.Comment, 0, len(rsp.GetComments()))
	for _, comment := range rsp.GetComments() {
		comments = append(comments, *dto.TransformProtoCommentToDTO(comment))
	}

	listCommentsDTO := &dto.ListCommentsResponse{
		Comments: comments,
		Pagination: dto.PaginationResponse{
			Offset:  int32(offset),
			Limit:   int32(limit),
			Records: rsp.GetPagination().GetTotalSize(),
		},
	}

	c.JSON(http.StatusOK, listCommentsDTO)
}

func (ph *PostHandler) CreateLike(c *gin.Context) {
	// Get post id from path
	postID := c.Param("postId")
	if postID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, goerrors.BadRequest)
	}

	req := &postv1.LikePostRequest{
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
		case codes.AlreadyExists:
			returnErr = goerrors.AlreadyLiked
		}

		ph.logger.Errorf("error in upstream call uh.postService.LikePost: %v", err)
		c.JSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
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

	req := &postv1.UnlikePostRequest{
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
		c.JSON(returnErr.HttpStatus, &dto.ErrorDTO{Error: returnErr})
		return
	}

	c.Status(http.StatusNoContent)
}
