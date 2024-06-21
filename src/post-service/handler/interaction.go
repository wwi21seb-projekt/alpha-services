package handler

import (
	"context"
	"errors"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"time"
)

type interactionService struct {
	logger        *zap.SugaredLogger
	tracer        trace.Tracer
	db            *db.DB
	profileClient pbUser.UserServiceClient
	pb.UnimplementedInteractionServiceServer
}

func NewInteractionService(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient) pb.InteractionServiceServer {
	return &interactionService{
		logger:        logger,
		tracer:        otel.GetTracerProvider().Tracer("post-service"),
		db:            db,
		profileClient: profileClient,
	}
}

func (is *interactionService) LikePost(ctx context.Context, req *pb.LikePostRequest) (*pbCommon.Empty, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	queryBuilder := psql.Insert("likes").Columns("post_id", "liked_at", "username").Values(req.PostId, time.Now().Format(time.RFC3339), authenticatedUsername)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, err
	}

	conn, err := is.db.Pool.Acquire(ctx)
	if err != nil {
		is.logger.Errorf("is.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgerrcode.UniqueViolation:
				return nil, status.Errorf(codes.AlreadyExists, "User already liked this post")
			case pgerrcode.ForeignKeyViolation:
				return nil, status.Errorf(codes.NotFound, "Post not found")
			default:
				is.logger.Errorw("Error executing SQL query", "error", err)
			}
			return nil, status.Errorf(codes.InvalidArgument, "Syntax error in query: %v", err)
		}
	}

	return &pbCommon.Empty{}, nil
}

func (is *interactionService) UnlikePost(ctx context.Context, req *pb.UnlikePostRequest) (*pbCommon.Empty, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	queryBuilder := psql.Delete("likes").Where(sq.Eq{"post_id": req.PostId, "username": authenticatedUsername})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, err
	}

	conn, err := is.db.Pool.Acquire(ctx)
	if err != nil {
		is.logger.Errorf("is.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	commandTag, err := conn.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgerrcode.ForeignKeyViolation:
				return nil, status.Errorf(codes.NotFound, "Post not found")
			default:
				is.logger.Errorw("Error executing SQL query", "error", err)
			}
		}
		return nil, status.Errorf(codes.Internal, "Error executing SQL query: %v", err)
	}

	// Check if any rows were affected
	if commandTag.RowsAffected() == 0 {
		return nil, status.Errorf(codes.NotFound, "User has not liked this post")
	}

	return &pbCommon.Empty{}, nil
}

func (is *interactionService) CreateComment(ctx context.Context, req *pb.CreateCommentRequest) (*pb.Comment, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get user data
	profiles, err := is.profileClient.ListUsers(ctx, &pbUser.ListUsersRequest{Usernames: []string{authenticatedUsername}})
	if err != nil {
		is.logger.Errorw("Error getting user data", "error", err)
		return nil, status.Errorf(codes.Internal, "Error getting user data: %v", err)
	}

	comment := &pb.Comment{
		CommentId: uuid.New().String(),
		Author: &pbUser.User{
			Username: profiles.GetUsers()[0].GetUsername(),
			Nickname: profiles.GetUsers()[0].GetNickname(),
			Picture:  profiles.GetUsers()[0].GetPicture(),
		},
		Content:      req.Content,
		CreationDate: time.Now().Format(time.RFC3339),
	}

	queryBuilder := psql.Insert("comments").Columns("comment_id", "post_id", "created_at", "author_name", "content").
		Values(comment.CommentId, req.GetPostId(), comment.GetCreationDate(), authenticatedUsername, comment.GetContent())

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building SQL query", "error", err)
		return nil, err
	}

	conn, err := is.db.Pool.Acquire(ctx)
	if err != nil {
		is.logger.Errorf("is.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgerrcode.ForeignKeyViolation:
				return nil, status.Errorf(codes.NotFound, "Post not found")
			default:
				is.logger.Errorw("Error executing SQL query", "error", err)

			}
			return nil, status.Errorf(codes.InvalidArgument, "Syntax error in query: %v", err)
		}

		is.logger.Errorw("Error executing SQL query", "error", err)
		return nil, status.Errorf(codes.Internal, "Error executing SQL query: %v", err)
	}

	return comment, nil
}

func (is *interactionService) ListComments(ctx context.Context, req *pb.ListCommentsRequest) (*pb.ListCommentsResponse, error) {
	// Base query builder
	baseQueryBuilder := psql.Select().From("comments").
		Where(sq.Eq{"post_id": req.GetPostId()})

	// Count query builder
	countQueryBuilder := baseQueryBuilder.Columns("COUNT(*)")
	countQuery, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building count SQL query", "error", err)
		return nil, status.Errorf(codes.Internal, "Error building count SQL query: %v", err)
	}

	// Data query builder with pagination
	dataQueryBuilder := baseQueryBuilder.Columns("comment_id", "created_at", "author_name", "content").
		Limit(uint64(req.Pagination.GetLimit())).
		Offset(uint64(req.Pagination.GetOffset())).
		OrderBy("created_at DESC")
	dataQuery, dataArgs, err := dataQueryBuilder.ToSql()
	if err != nil {
		is.logger.Errorw("Error building data SQL query", "error", err)
		return nil, status.Errorf(codes.Internal, "Error building data SQL query: %v", err)
	}

	conn, err := is.db.Pool.Acquire(ctx)
	if err != nil {
		is.logger.Errorf("is.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Execute count query to get total records
	var totalRecords int
	err = conn.QueryRow(ctx, countQuery, countArgs...).Scan(&totalRecords)
	if err != nil {
		is.logger.Errorw("Error executing count SQL query", "error", err)
		return nil, status.Errorf(codes.Internal, "Error executing count SQL query: %v", err)
	}

	// Execute data query to get comments
	rows, err := conn.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		is.logger.Errorw("Error executing data SQL query", "error", err)
		return nil, status.Errorf(codes.Internal, "Error executing data SQL query: %v", err)
	}
	defer rows.Close()

	comments := make([]*pb.Comment, 0)
	commentToAuthor := make(map[string]string)
	for rows.Next() {
		comment := &pb.Comment{
			Author: &pbUser.User{}, // Ensure Author is not nil
		}
		var creationDate pgtype.Timestamptz
		err = rows.Scan(&comment.CommentId, &creationDate, &comment.Author.Username, &comment.Content)
		if err != nil {
			is.logger.Errorw("Error scanning row", "error", err)
			return nil, status.Errorf(codes.Internal, "Error scanning row: %v", err)
		}
		comment.CreationDate = creationDate.Time.Format(time.RFC3339)
		commentToAuthor[comment.CommentId] = comment.Author.Username

		comments = append(comments, comment)
	}

	uniqueProfiles := make(map[string]struct{})
	for _, author := range commentToAuthor {
		uniqueProfiles[author] = struct{}{}
	}

	ids := make([]string, 0, len(uniqueProfiles))
	for id := range uniqueProfiles {
		ids = append(ids, id)
	}

	profiles, err := is.profileClient.ListUsers(ctx, &pbUser.ListUsersRequest{Usernames: ids})
	if err != nil {
		is.logger.Errorw("Error getting user data", "error", err)
		return nil, status.Errorf(codes.Internal, "Error getting user data: %v", err)
	}

	profileMap := make(map[string]*pbUser.User)
	for _, profile := range profiles.Users {
		profileMap[profile.Username] = &pbUser.User{
			Username: profile.GetUsername(),
			Nickname: profile.GetNickname(),
			Picture:  profile.GetPicture(),
		}
	}

	for _, comment := range comments {
		if profile, exists := profileMap[comment.Author.Username]; exists {
			comment.Author = profile
		} else {
			is.logger.Warnw("No profile found for user", "username", comment.Author.Username)
		}
	}

	return &pb.ListCommentsResponse{
		Comments: comments,
		Pagination: &pbCommon.Pagination{
			Offset:  req.Pagination.GetOffset(),
			Limit:   req.Pagination.GetLimit(),
			Records: int32(totalRecords),
		},
	}, nil
}
