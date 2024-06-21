package handler

import (
	"context"
	"errors"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

func (ps *postService) ListPosts(ctx context.Context, request *pb.ListPostsRequest) (*pb.ListPostsResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		ps.logger.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	baseQueryBuilder := psql.Select().
		From("posts p")

	if request.FeedType == pb.FeedType_FEED_TYPE_USER {

		if request.Username != nil && *request.Username == "" {
			return nil, status.Error(codes.InvalidArgument, "Username cannot be empty")
		}

		username := *request.Username
		likedByUser := sq.Eq{"l.username": username}
		commentedByUser := sq.Eq{"c.author_name": username}
		authoredByUser := sq.Eq{"p.author_name": username}
		baseQueryBuilder = baseQueryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Or{likedByUser, commentedByUser, authoredByUser})

	}

	if request.FeedType == pb.FeedType_FEED_TYPE_PERSONAL {
		authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
		newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

		subCtx, subSpan := ps.tracer.Start(newCTX, "GetSubscriptions")
		subscriptions, err := ps.subscription.ListSubscriptions(subCtx, &pbUser.ListSubscriptionsRequest{
			Username:         authenticatedUsername,
			SubscriptionType: pbUser.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING,
			Pagination:       &pbCommon.PaginationRequest{Limit: 1000},
		})
		if err != nil {
			subSpan.End()
			ps.logger.Errorf("Error in subscriptionClient.ListSubscriptions: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to get subscriptions: %v", err)
		}
		subSpan.End()

		peopleIFollow := make([]string, 0)
		for _, sub := range subscriptions.Subscriptions {
			peopleIFollow = append(peopleIFollow, sub.Username)
		}
		peopleIFollow = append(peopleIFollow, authenticatedUsername)

		authoredCondition := sq.Eq{"p.author_name": peopleIFollow}
		likedCondition := sq.Eq{"l.username": peopleIFollow}
		commentedCondition := sq.Eq{"c.author_name": peopleIFollow}

		baseQueryBuilder = baseQueryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Or{authoredCondition, likedCondition, commentedCondition})
	}

	if request.Hashtag != nil {
		baseQueryBuilder = baseQueryBuilder.Join("many_posts_has_many_hashtags h ON p.post_id = h.post_id_posts").
			Join("hashtags h2 ON h.hashtag_id_hashtags = h2.hashtag_id").
			Where(sq.Eq{"h2.content": request.Hashtag})
	}

	// Create the count query
	countQueryBuilder := baseQueryBuilder.
		Columns("COUNT(DISTINCT p.post_id)")

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

	if request.GetPagination() != nil && request.Pagination.GetLastPostId() != "" {
		baseQueryBuilder = baseQueryBuilder.Where("p.created_at < (SELECT created_at FROM posts WHERE post_id = ?)", request.Pagination.LastPostId)
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
		ps.logger.Errorf("ps.populateReposts failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to populate reposts: %v", err)
	}

	// Get the author profile for each post
	err = ps.populateAuthors(ctx, posts)
	if err != nil {
		ps.logger.Errorf("ps.populateAuthors failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to populate author profiles: %v", err)
	}

	resp := &pb.ListPostsResponse{
		Posts: posts,
		Pagination: &pb.PostPagination{
			LastPostId: request.Pagination.LastPostId,
			Limit:      request.Pagination.Limit,
			Records:    int32(totalRecords),
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

func (ps *postService) populateAuthors(ctx context.Context, posts []*pb.Post) error {
	// Create a set to hold unique author usernames
	uniqueAuthors := make(map[string]struct{})

	// Collect all unique author usernames
	for _, post := range posts {
		if post.Author != nil && post.Author.Username != "" {
			uniqueAuthors[post.Author.Username] = struct{}{}
		}
	}

	// Convert the set keys to a slice
	usernames := make([]string, 0, len(uniqueAuthors))
	for username := range uniqueAuthors {
		usernames = append(usernames, username)
	}

	// Make a gRPC call to fetch user details
	profiles, err := ps.profileClient.ListUsers(ctx, &pbUser.ListUsersRequest{Usernames: usernames})
	if err != nil {
		ps.logger.Errorf("ps.profileClient.ListUsers failed: %v", err)
		return status.Errorf(codes.Internal, "Failed to get user profiles: %v", err)
	}

	// Create a map from username to profile for quick lookup
	profileMap := make(map[string]*pbUser.PublicUser)
	for _, profile := range profiles.Users {
		profileMap[profile.Username] = &pbUser.PublicUser{
			Username: profile.GetUsername(),
			Nickname: profile.GetNickname(),
			Picture:  profile.GetPicture(),
		}
	}

	// Populate the author details in the posts
	for _, post := range posts {
		if profile, exists := profileMap[post.Author.Username]; exists {
			post.Author = profile
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
	if repostId != nil {
		post.Repost, _, err = ps.retrievePost(ctx, &pb.GetPostRequest{PostId: *repostId}, conn)
		if err != nil {
			ps.logger.Errorf("ps.GetPost failed: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get repost: %v", err)
		}
	}

	return post, nil
}

func (ps *postService) retrievePost(ctx context.Context, request *pb.GetPostRequest, conn *pgxpool.Conn) (*pb.Post, *string, error) {
	query := psql.Select("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		From("posts p").
		Where(sq.Eq{"p.post_id": request.PostId})

	queryString, args, err := query.ToSql()
	if err != nil {
		ps.logger.Errorf("query.ToSql() failed: %v", err)
		return nil, nil, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	row := conn.QueryRow(ctx, queryString, args...)

	ps.logger.Infof("Row: %v", row)

	post := &pb.Post{Author: &pbUser.PublicUser{}}
	var createdAt time.Time
	location := &pb.Location{}
	var repostID *string
	if err = row.Scan(&post.PostId, &post.Author.Username, &post.Content, &createdAt, &location.Longitude, &location.Latitude, &location.Accuracy, &repostID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgerrcode.IsSyntaxErrororAccessRuleViolation(pgErr.Code) || pgerrcode.IsDataException(pgErr.Code)) {
			return nil, nil, status.Errorf(codes.InvalidArgument, "Syntax error in query: %v", err)
		}

		ps.logger.Errorf("row.Scan failed: %v", err)
		return nil, nil, status.Errorf(codes.Internal, "Failed to scan row: %v", err)
	}

	ps.logger.Infof("Post: %v", post)

	post.CreationDate = createdAt.Format(time.RFC3339)

	if location.Longitude != nil && location.Latitude != nil && location.Accuracy != nil {
		post.Location = location
	}

	return post, repostID, nil
}

func (ps *postService) CreatePost(ctx context.Context, request *pb.CreatePostRequest) (*pb.Post, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get Author Profile from User-Service
	selectCtx, selectSpan := ps.tracer.Start(ctx, "SelectUserData")
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

	// Get repost if it exists
	repostCTX, repostSpan := ps.tracer.Start(ctx, "GetRepost")
	var repostID *string
	if request.RepostedPostId != nil && *request.RepostedPostId != "" {
		post.Repost, err = ps.GetPost(repostCTX, &pb.GetPostRequest{PostId: *request.RepostedPostId})
		if err != nil {
			repostSpan.SetAttributes(attribute.String("error", err.Error()))
			repostSpan.End()

			if status.Code(err) == codes.NotFound {
				return nil, status.Errorf(codes.InvalidArgument, "repost does not exist: %v", err)
			}

			ps.logger.Errorf("Error in GetPost: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to get repost: %v", err)
		}
		repostID = &post.Repost.PostId
		ps.logger.Infof("Repost: %v", post.Repost)
	}

	// Start transaction
	tx, err := ps.db.Begin(ctx)
	if err != nil {
		ps.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer ps.db.Rollback(ctx, tx)

	err = ps.insertPost(ctx, tx, post, repostID)
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
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get the post
	post, err := ps.GetPost(ctx, req)
	if err != nil {
		ps.logger.Errorf("ps.GetPost failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to get post: %v", err)
	}

	// Check if the authenticated user is the author of the post
	if post.Author.Username != authenticatedUsername {
		return nil, status.Error(codes.PermissionDenied, "You are not the author of this post")
	}

	// Start transaction
	tx, err := ps.db.Begin(ctx)
	if err != nil {
		ps.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer ps.db.Rollback(ctx, tx)

	// Delete the post
	queryString := "DELETE FROM post_service.posts WHERE post_id = $1"
	if _, err := tx.Exec(ctx, queryString, post.PostId); err != nil {
		ps.logger.Errorf("tx.Exec failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to delete post: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		ps.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func (ps *postService) insertPost(ctx context.Context, tx pgx.Tx, post *pb.Post, repostID *string) error {
	query := psql.Insert("posts").
		Columns("post_id", "author_name", "content", "created_at", "longitude", "latitude", "accuracy", "repost_post_id").
		Values(post.PostId, post.Author.Username, post.Content, post.CreationDate, &post.Location.Longitude, &post.Location.Latitude, &post.Location.Accuracy, &repostID)

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
