package handler

import (
	"context"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"regexp"
	"strings"
	"time"

	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

var hashtagRegex = regexp.MustCompile(`#\w+`)
var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type postService struct {
	logger        *zap.SugaredLogger
	tracer        trace.Tracer
	db            *db.DB
	profileClient pbUser.UserServiceClient
	subscription  pbUser.SubscriptionServiceClient
	pb.UnimplementedPostServiceServer
}

func NewPostServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient, subscription pbUser.SubscriptionServiceClient) pb.PostServiceServer {
	return &postService{
		logger:        logger,
		tracer:        otel.GetTracerProvider().Tracer("post-service"),
		db:            db,
		profileClient: profileClient,
		subscription:  subscription,
	}
}

func (ps *postService) SearchPosts(ctx context.Context, empty *pb.SearchPostsRequest) (*pb.SearchPostsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (ps *postService) ListPosts(ctx context.Context, request *pb.ListPostsRequest) (*pb.SearchPostsResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		ps.logger.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	baseQueryBuilder := psql.Select().
		From("posts p")

	if request.Hashtag != nil {
		baseQueryBuilder = baseQueryBuilder.Join("many_posts_has_many_hashtags h ON p.post_id = h.post_id_posts").
			Where(sq.Eq{"h.hashtag_id_hashtags": request.Hashtag})
	}

	if request.LikedBy != nil || request.CommentedBy != nil || request.Author != nil {
		likedByUser := sq.Eq{"l.username": request.LikedBy}
		commentedByUser := sq.Eq{"c.author_name": request.CommentedBy}
		authoredByUser := sq.Eq{"p.author_name": request.Author}
		baseQueryBuilder = baseQueryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Or{likedByUser, commentedByUser, authoredByUser})
	}

	if request.Pagination.LastPostId != "" {
		baseQueryBuilder = baseQueryBuilder.Where("p.created_at < (SELECT created_at FROM posts WHERE post_id = ?)", request.Pagination.LastPostId)
	}

	// Create the count query
	countQueryBuilder := baseQueryBuilder.
		Columns("COUNT(*)")

	countQueryString, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorf("countQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build count query: %v", err)
	}

	var totalRecords int
	err = conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords)
	if err != nil {
		ps.logger.Errorf("conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to execute count query: %v", err)
	}

	dataQueryBuilder := baseQueryBuilder.
		Columns("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		OrderBy("p.created_at DESC").
		Limit(uint64(request.Pagination.Limit))

	dataQueryString, dataArgs, err := dataQueryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorf("baseQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	rows, err := conn.Query(ctx, dataQueryString, dataArgs...)
	if err != nil {
		ps.logger.Errorf("conn.Query(ctx, dataQueryString, dataArgs...) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to query data: %v", err)
	}

	var posts []*pb.Post
	repostIDs := make(map[string]string) // Map of post_id -> repost_post_id
	for rows.Next() {
		post := &pb.Post{
			Author:   &pbUser.PublicUser{},
			Location: &pb.Location{},
		}
		var creationDate pgtype.Timestamptz
		var repostID *string
		if err = rows.Scan(&post.PostId, &post.Author.Username, &post.Content, &creationDate, &post.Location.Longitude, &post.Location.Latitude, &post.Location.Accuracy, &repostID); err != nil {
			ps.logger.Errorf("rows.Scan failed: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to scan row: %v", err)
		}
		post.CreationDate = creationDate.Time.Format(time.RFC3339)

		if repostID != nil {
			repostIDs[post.PostId] = *repostID
		}

		if post.Location.Longitude == nil || post.Location.Latitude == nil || post.Location.Accuracy == nil {
			post.Location = nil
		}

		posts = append(posts, post)
	}

	// Get the reposts (one level) if they exist and populate the posts
	err = ps.populateReposts(ctx, conn, posts, repostIDs)
	if err != nil {
		return nil, err
	}

	// Get the author profile for each post

	// newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authorName.String) // Dont use another user as context (but it's funny)
	/*
		author, err := ps.profileClient.ListUsers(newCTX, &pbUser.GetUserRequest{Username: authorName.String})
		if err != nil {
			ps.logger.Errorf("ps.profileClient.GetUser failed: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get author profile: %v", err)
		}
	*/
	resp := &pb.SearchPostsResponse{
		Posts: posts,
		Pagination: &pbCommon.Pagination{
			Limit:   request.Pagination.Limit,
			Records: int32(totalRecords),
		},
	}

	return resp, nil
}

func (ps *postService) populateReposts(ctx context.Context, conn *pgxpool.Conn, posts []*pb.Post, repostIDs map[string]string) error {
	// Create a set to hold unique repost IDs
	uniqueIDs := make(map[string]struct{})

	// Iterate over the repostIDs map and add each value to the set
	for _, id := range repostIDs {
		uniqueIDs[id] = struct{}{}
	}

	// Convert the set keys to a slice
	ids := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		ids = append(ids, id)
	}

	repostQueryBuilder := psql.Select("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy").
		From("posts p").
		Where(sq.Eq{"p.post_id": ids})

	repostQueryString, repostArgs, err := repostQueryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorf("repostQueryBuilder.ToSql() failed: %v", err)
		return status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	rows, err := conn.Query(ctx, repostQueryString, repostArgs...)
	if err != nil {
		ps.logger.Errorf("conn.Query(ctx, repostQueryString, repostArgs...) failed: %v", err)
		return status.Errorf(codes.Internal, "Failed to query data: %v", err)
	}

	reposts := make(map[string]*pb.Post) // Map of repost_post_id -> repost
	for rows.Next() {
		repost := &pb.Post{Author: &pbUser.PublicUser{}}
		location := &pb.Location{}
		var creationDate pgtype.Timestamptz
		if err = rows.Scan(&repost.PostId, &repost.Author.Username, &repost.Content, &creationDate, &location.Longitude, &location.Latitude, &location.Accuracy); err != nil {
			ps.logger.Errorf("rows.Scan failed: %v", err)
			return status.Errorf(codes.Internal, "Failed to scan row: %v", err)
		}

		repost.CreationDate = creationDate.Time.Format(time.RFC3339)
		reposts[repost.PostId] = repost

		if location.Longitude != nil && location.Latitude != nil && location.Accuracy != nil {
			repost.Location = location
		}
	}

	// Iterate through the posts and populate the reposts
	for _, post := range posts {
		if repostID, ok := repostIDs[post.PostId]; ok {
			post.Repost = reposts[repostID]
		}
	}

	return nil
}

func (ps *postService) GetPost(ctx context.Context, request *pb.GetPostRequest) (*pb.Post, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		ps.logger.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	post, repostId, err := ps.retrievePost(ctx, request, conn)
	if err != nil {
		ps.logger.Errorf("ps.retrievePost failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to retrieve post: %v", err)
	}

	// Get the repost (one level) if it exists
	if repostId.Valid {
		post.Repost, _, err = ps.retrievePost(ctx, &pb.GetPostRequest{PostId: repostId.String}, conn)
		if err != nil {
			ps.logger.Errorf("ps.GetPost failed: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get repost: %v", err)
		}
	}

	return post, nil
}

func (ps *postService) retrievePost(ctx context.Context, request *pb.GetPostRequest, conn *pgxpool.Conn) (*pb.Post, pgtype.Text, error) {
	query := psql.Select("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		From("posts p").
		Where(sq.Eq{"p.post_id": request.PostId})

	queryString, args, err := query.ToSql()
	if err != nil {
		ps.logger.Errorf("query.ToSql() failed: %v", err)
		return nil, pgtype.Text{}, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	row := conn.QueryRow(ctx, queryString, args...)

	post := &pb.Post{}
	var createdAt time.Time
	var longitude, latitude pgtype.Float8
	var accuracy pgtype.Int4
	var repostId pgtype.Text
	if err = row.Scan(&post.PostId, &post.Author.Username, &post.Content, &createdAt, &longitude, &latitude, &accuracy, &repostId); err != nil {
		ps.logger.Errorf("row.Scan failed: %v", err)
		return nil, pgtype.Text{}, status.Errorf(codes.Internal, "Failed to scan row: %v", err)
	}
	return post, repostId, nil
}

func (ps *postService) CreatePost(ctx context.Context, request *pb.CreatePostRequest) (*pb.Post, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get Author Profile from User-Service
	selectCtx, selectSpan := ps.tracer.Start(newCTX, "SelectUserData")
	users, err := ps.profileClient.ListUsers(selectCtx, &pbUser.ListUsersRequest{Usernames: []string{authenticatedUsername}})
	if err != nil {
		selectSpan.End()
		ps.logger.Errorf("Error in profileClient.GetUser: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to get author profile: %v", err)
	}
	selectSpan.End()

	post := &pb.Post{
		PostId:       uuid.New().String(),
		CreationDate: time.Now().Format(time.RFC3339),
		Author:       users.Users[0],
		Content:      request.Content,
		Location:     request.Location,
		Liked:        false,
		Likes:        0,
	}

	// Start transaction
	tx, err := ps.db.Begin(ctx)
	if err != nil {
		ps.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer ps.db.Rollback(ctx, tx)

	if request.RepostedPostId != nil {
		// Get the repost
		repost, err := ps.GetPost(ctx, &pb.GetPostRequest{PostId: *request.RepostedPostId})
		if err != nil {
			ps.logger.Errorf("Error in GetPost: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to get repost: %v", err)
		}

		post.Repost = repost
	}

	err = ps.insertPost(ctx, tx, post)
	if err != nil {
		ps.logger.Errorf("Error in insertPost: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert post: %v", err)
	}

	err = ps.insertHashtags(ctx, tx, post)
	if err != nil {
		ps.logger.Errorf("Error in insertHashtags: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert hashtags: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		ps.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return post, nil
}

func (ps *postService) DeletePost(ctx context.Context, req *pb.GetPostRequest) (*pbCommon.Empty, error) {
	panic("implement me")
}

func (ps *postService) insertPost(ctx context.Context, tx pgx.Tx, post *pb.Post) error {
	query := psql.Insert("posts").
		Columns("post_id", "author_name", "content", "created_at",
			"longitude", "latitude", "accuracy").
		Values(post.PostId, post.Author.Username, post.Content, post.CreationDate,
			post.Location.Longitude, post.Location.Latitude, post.Location.Accuracy)

	queryString, args, _ := query.ToSql()
	_, err := tx.Exec(ctx, queryString, args...)

	return err
}

func (ps *postService) insertHashtags(ctx context.Context, tx pgx.Tx, post *pb.Post) error {
	hashtags := hashtagRegex.FindAllString(post.Content, -1)
	for _, hashtag := range hashtags {
		hashtagId := uuid.New()

		queryString := `INSERT INTO post_service.hashtags (hashtag_id, content) VALUES($1, $2) 
					ON CONFLICT (content) DO UPDATE SET content=hashtags.content 
					RETURNING hashtag_id`
		if err := tx.QueryRow(ctx, queryString, hashtagId, strings.ToLower(hashtag)).Scan(&hashtagId); err != nil {
			return err
		}

		queryString = "INSERT INTO post_service.many_posts_has_many_hashtags (post_id_posts, hashtag_id_hashtags) VALUES($1, $2)"
		if _, err := tx.Exec(ctx, queryString, post.PostId, hashtagId); err != nil {
			return err
		}
	}
	return nil
}
