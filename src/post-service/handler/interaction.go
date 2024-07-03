package handler

import (
	"context"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/wwi21seb-projekt/alpha-services/src/post-service/schema"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type interactionService struct {
	postv1.UnimplementedInteractionServiceServer
	logger        *zap.SugaredLogger
	tracer        trace.Tracer
	db            *db.DB
	profileClient userv1.UserServiceClient
}

func NewInteractionService(logger *zap.SugaredLogger, db *db.DB, profileClient userv1.UserServiceClient) postv1.InteractionServiceServer {
	return &interactionService{
		logger:        logger,
		tracer:        otel.GetTracerProvider().Tracer("post-service"),
		db:            db,
		profileClient: profileClient,
	}
}

func (is *interactionService) LikePost(ctx context.Context, req *postv1.LikePostRequest) (*postv1.LikePostResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	queryBuilder := psql.
		Insert("likes").
		Columns("post_id", "liked_at", "username").
		Values(req.PostId, time.Now().Format(time.RFC3339), authenticatedUsername)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error building SQL query")
	}

	conn, err := is.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	_, err = is.db.Exec(ctx, conn, query, args...)
	if err != nil {
		return nil, err
	}

	return &postv1.LikePostResponse{}, nil
}

func (is *interactionService) UnlikePost(ctx context.Context, req *postv1.UnlikePostRequest) (*postv1.UnlikePostResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	queryBuilder := psql.
		Delete("likes").
		Where(sq.Eq{"post_id": req.PostId, "username": authenticatedUsername})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error building SQL query")
	}

	conn, err := is.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	commandTag, err := is.db.Exec(ctx, conn, query, args...)
	if err != nil {
		return nil, err
	}

	// Check if any rows were affected
	if commandTag.RowsAffected() == 0 {
		return nil, status.Errorf(codes.NotFound, "like not found")
	}

	return &postv1.UnlikePostResponse{}, nil
}

func (is *interactionService) CreateComment(ctx context.Context, req *postv1.CreateCommentRequest) (*postv1.CreateCommentResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Fetch user data
	userCTX, userSpan := is.tracer.Start(ctx, "Fetch user data")
	profiles, err := is.profileClient.ListUsers(userCTX, &userv1.ListUsersRequest{Usernames: []string{authenticatedUsername}})
	if err != nil {
		userSpan.End()
		is.logger.Errorw("Error getting user data", "error", err)
		return nil, status.Errorf(codes.Internal, "Error getting user data: %v", err)
	}
	userSpan.End()

	comment := &schema.Comment{
		CommentID:  uuid.New().String(),
		Content:    req.GetContent(),
		CreatedAt:  time.Now(),
		AuthorName: profiles.GetUsers()[0].GetUsername(),
		PostID:     req.GetPostId(),
	}

	queryBuilder := psql.
		Insert("comments").
		Columns("comment_id", "content", "created_at", "author_name", "post_id").
		Values(comment.CommentID, comment.Content, comment.CreatedAt, comment.AuthorName, comment.PostID)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, err
	}

	conn, err := is.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	_, err = is.db.Exec(ctx, conn, query, args...)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateCommentResponse{
		CommentId:    comment.CommentID,
		Author:       profiles.GetUsers()[0],
		Content:      comment.Content,
		CreationDate: comment.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (is *interactionService) ListComments(ctx context.Context, req *postv1.ListCommentsRequest) (*postv1.ListCommentsResponse, error) {
	// Check if post exists
	postCTX, postSpan := is.tracer.Start(ctx, "Fetch post data")
	existsQuery, existsArgs, err := psql.
		Select("COUNT(*)").
		From("posts").
		Where("post_id = ?", req.GetPostId()).
		ToSql()
	if err != nil {
		postSpan.End()
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error building SQL query")
	}

	conn, err := is.db.Acquire(ctx)
	if err != nil {
		is.logger.Errorw("Error acquiring connection", "error", err)
		return nil, err
	}
	defer conn.Release()

	var exists int
	err = conn.QueryRow(postCTX, existsQuery, existsArgs...).Scan(&exists)
	if err != nil {
		postSpan.End()
		is.logger.Errorw("Error executing SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error executing SQL query")
	}
	postSpan.End()

	if exists == 0 {
		return nil, status.Errorf(codes.NotFound, "post not found")
	}

	baseQueryBuilder := psql.
		Select().
		From("comments").
		Where(sq.Eq{"post_id": req.GetPostId()})

	countQueryBuilder := baseQueryBuilder.Columns("COUNT(*)")

	countQuery, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building count SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error building count SQL query")
	}

	limit := int(req.Pagination.GetPageSize())
	if limit <= 0 {
		limit = 10
	}
	offset, err := strconv.Atoi(req.Pagination.GetPageToken())
	if err != nil {
		is.logger.Debugw("Error parsing page token", "error", err)
		offset = 0
	}

	// Data query builder with pagination
	dataQueryBuilder := baseQueryBuilder.Columns("comment_id", "created_at", "author_name", "content", "post_id").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		OrderBy("created_at DESC")

	dataQuery, dataArgs, err := dataQueryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building data SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error building data SQL query")
	}

	// Execute count query to get total records
	var totalRecords int
	err = conn.QueryRow(ctx, countQuery, countArgs...).Scan(&totalRecords)
	if err != nil {
		is.logger.Errorw("Error executing count SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error executing count SQL query")
	}

	// Execute data query to get comments
	rows, err := conn.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		is.logger.Errorw("Error executing data SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Error executing data SQL query")
	}
	defer rows.Close()

	comments, err := pgx.CollectRows(rows, pgx.RowToStructByName[schema.Comment])
	if err != nil {
		is.logger.Errorw("Error scanning rows", "error", err)
		return nil, status.Error(codes.Internal, "Error scanning rows")
	}

	authorNames := make([]string, 0, len(comments))
	for _, comment := range comments {
		authorNames = append(authorNames, comment.AuthorName)
	}

	authorsCTX, authorsSpan := is.tracer.Start(ctx, "Fetch author data")
	authorProfiles, err := is.profileClient.ListUsers(authorsCTX, &userv1.ListUsersRequest{Usernames: authorNames})
	if err != nil {
		authorsSpan.End()
		is.logger.Errorw("Error getting user data", "error", err)
		return nil, status.Error(codes.Internal, "Error getting user data")
	}
	authorsSpan.End()

	authorMap := make(map[string]*userv1.User)
	for _, profile := range authorProfiles.GetUsers() {
		authorMap[profile.GetUsername()] = profile
	}

	resp := &postv1.ListCommentsResponse{
		Comments: make([]*postv1.CreateCommentResponse, 0, len(comments)),
		Pagination: &commonv1.PaginationResponse{
			NextPageToken: strconv.Itoa(offset + limit),
			TotalSize:     int32(totalRecords),
		},
	}

	for _, comment := range comments {
		resp.Comments = append(resp.Comments, &postv1.CreateCommentResponse{
			CommentId:    comment.CommentID,
			Author:       authorMap[comment.AuthorName],
			Content:      comment.Content,
			CreationDate: comment.CreatedAt.Format(time.RFC3339),
		})
	}

	return resp, nil
}
