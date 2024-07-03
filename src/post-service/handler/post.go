package handler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwi21seb-projekt/alpha-services/src/post-service/schema"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
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

var (
	hashtagRegex  = regexp.MustCompile(`#\w+`)
	psql          = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	basePostQuery = psql.Select().From("posts p")
)

type postService struct {
	logger        *zap.SugaredLogger
	tracer        trace.Tracer
	db            *db.DB
	profileClient userv1.UserServiceClient
	subscription  userv1.SubscriptionServiceClient
	imageClient   imagev1.ImageServiceClient
	postv1.UnimplementedPostServiceServer
}

func NewPostServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient userv1.UserServiceClient, subscription userv1.SubscriptionServiceClient, imageClient imagev1.ImageServiceClient) postv1.PostServiceServer {
	return &postService{
		logger:        logger,
		tracer:        otel.GetTracerProvider().Tracer("post-service"),
		db:            db,
		profileClient: profileClient,
		subscription:  subscription,
		imageClient:   imageClient,
	}
}

func (ps *postService) ListPosts(ctx context.Context, req *postv1.ListPostsRequest) (*postv1.ListPostsResponse, error) {
	queryBuilder := basePostQuery

	if len(req.GetPostIds()) > 0 {
		queryBuilder = queryBuilder.Where(sq.Eq{"p.post_id": req.GetPostIds()})
	}

	var authenticatedUsername string
	if req.GetFeedType() == postv1.FeedType_FEED_TYPE_USER {
		if req.GetUsername() == "" {
			return nil, status.Error(codes.InvalidArgument, "username cannot be empty")
		}

		// Check if user exists
		userCTX, userSpan := ps.tracer.Start(ctx, "GetUser")
		resp, err := ps.profileClient.ListUsers(userCTX, &userv1.ListUsersRequest{Usernames: []string{*req.Username}})
		if err != nil {
			userSpan.End()
			ps.logger.Errorw("Error in profileClient.GetUser while checking if user exists", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to check if user exists")
		}
		userSpan.End()

		if len(resp.GetUsers()) == 0 {
			ps.logger.Debugw("User does not exist", zap.String("username", *req.Username))
			return nil, status.Error(codes.NotFound, "user does not exist")
		}

		queryBuilder = queryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Eq{"p.author_name": req.GetUsername()})
	}

	if req.GetFeedType() == postv1.FeedType_FEED_TYPE_PERSONAL {
		authenticatedUsername = metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
		newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

		subCtx, subSpan := ps.tracer.Start(newCTX, "ListSubscriptions")
		subscriptions, err := ps.subscription.ListSubscriptions(subCtx, &userv1.ListSubscriptionsRequest{
			Username:         authenticatedUsername,
			SubscriptionType: userv1.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING,
			Pagination:       &commonv1.PaginationRequest{PageSize: 1000},
		})
		if err != nil {
			subSpan.End()
			ps.logger.Errorw("Error while getting subscriptions", zap.Error(err))
			return nil, err
		}
		subSpan.End()

		peopleIFollow := make([]string, 0)
		for _, sub := range subscriptions.Subscriptions {
			peopleIFollow = append(peopleIFollow, sub.Username)
		}

		authoredCondition := sq.Eq{"p.author_name": peopleIFollow}
		likedCondition := sq.Eq{"l.username": peopleIFollow}
		commentedCondition := sq.Eq{"c.author_name": peopleIFollow}

		queryBuilder = queryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Or{authoredCondition, likedCondition, commentedCondition})
	}

	if req.GetHashtag() != "" {
		queryHashtag := req.GetHashtag()
		// If there is a # in the hashtag, keep it, if not, add it
		if !strings.HasPrefix(req.GetHashtag(), "#") {
			queryHashtag = "#" + req.GetHashtag()
		}
		queryBuilder = queryBuilder.
			Join("many_posts_has_many_hashtags h ON p.post_id = h.post_id_posts").
			Join("hashtags h2 ON h.hashtag_id_hashtags = h2.hashtag_id").
			Where(sq.Eq{"h2.content": queryHashtag})
	}

	// Create the count query
	countQueryBuilder := queryBuilder.
		Columns("COUNT(DISTINCT p.post_id)")

	countQueryString, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building count query", "error", err)
		return nil, status.Error(codes.Internal, "failed to build count query")
	}

	dataQueryBuilder := queryBuilder.
		Columns("DISTINCT p.*").
		OrderBy("p.created_at DESC").
		Limit(uint64(req.Pagination.GetPageSize()))

	var offset int
	if req.Pagination.GetPageToken() != "" {
		offset, err = strconv.Atoi(req.Pagination.GetPageToken())
		if err != nil {
			dataQueryBuilder = dataQueryBuilder.Where("p.created_at < (SELECT created_at FROM posts WHERE post_id = ?)", req.Pagination.GetPageToken())
		} else {
			dataQueryBuilder = dataQueryBuilder.Offset(uint64(offset))
		}
	}

	conn, err := ps.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	var totalRecords int
	if len(req.GetPostIds()) == 0 {
		err = conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords)
		if err != nil {
			ps.logger.Errorw("Error executing count query", "error", err)
			return nil, status.Error(codes.Internal, "failed to get total records")
		}
	}

	protoPosts, err := ps.retrievePosts(ctx, conn, dataQueryBuilder, authenticatedUsername)
	if err != nil {
		return nil, err
	}

	protoPagination := &commonv1.PaginationResponse{
		TotalSize: int32(totalRecords),
	}

	if req.FeedType != postv1.FeedType_FEED_TYPE_USER || govalidator.IsUUIDv4(req.Pagination.GetPageToken()) {
		// If the protoPosts contain the last post, then the next page token is the last post's ID
		if len(protoPosts) > 0 {
			protoPagination.NextPageToken = protoPosts[len(protoPosts)-1].PostId
		} else {
			protoPagination.NextPageToken = fmt.Sprintf("%d", offset)
		}
	} else {
		offset, _ := strconv.Atoi(req.Pagination.GetPageToken())
		protoPagination.NextPageToken = fmt.Sprintf("%d", offset+len(protoPosts))
	}

	return &postv1.ListPostsResponse{
		Posts:      protoPosts,
		Pagination: protoPagination,
	}, nil
}

func (ps *postService) CreatePost(ctx context.Context, request *postv1.CreatePostRequest) (*postv1.CreatePostResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get Author Profile from User-Service
	selectCtx, selectSpan := ps.tracer.Start(ctx, "SelectUserData")
	users, err := ps.profileClient.ListUsers(selectCtx, &userv1.ListUsersRequest{Usernames: []string{authenticatedUsername}})
	if err != nil {
		selectSpan.End()
		if status.Code(err) == codes.PermissionDenied {
			return nil, err
		}
		ps.logger.Errorw("Error in GetUser", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get user data")
	}
	selectSpan.End()

	post := &schema.Post{
		PostID:       uuid.New().String(),
		Content:      request.Content,
		CreatedAt:    time.Now(),
		AuthorName:   authenticatedUsername,
		RepostPostID: request.RepostedPostId,
	}

	if request.Location != nil {
		post.Latitude = &request.Location.Latitude
		post.Longitude = &request.Location.Longitude
		post.Accuracy = &request.Location.Accuracy
	}

	conn, err := ps.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	// Get repost if it exists
	var repost *postv1.Post
	if request.GetRepostedPostId() != "" {
		queryBuilder := basePostQuery.Columns("p.*").Where(sq.Eq{"p.post_id": request.GetRepostedPostId()})
		posts, err := ps.retrievePosts(ctx, conn, queryBuilder, authenticatedUsername)
		if err != nil {
			return nil, err
		}
		if len(posts) == 0 {
			return nil, status.Errorf(codes.NotFound, "repostedPostId was not found")
		}
		repost = posts[0]
	}

	// Upload the image to image-service
	var picture *imagev1.Picture
	if request.Picture != nil {
		uploadResponse, err := ps.imageClient.UploadImage(ctx, &imagev1.UploadImageRequest{
			Image: *request.Picture,
			Name:  post.PostID,
		})
		if err != nil {
			ps.logger.Errorw("Error in imageClient.UploadImage", "error", err)
			return nil, err
		}

		picture = &imagev1.Picture{
			Width:  uploadResponse.Width,
			Height: uploadResponse.Height,
			Url: uploadResponse.Url,
		}

		post.PictureURL = &picture.Url
		post.PictureWidth = &picture.Width
		post.PictureHeight = &picture.Height
	}

	tx, err := ps.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer ps.db.RollbackTx(ctx, tx)

	queryString, args, _ := psql.Insert("posts").
		Columns("post_id", "author_name", "content", "created_at", "longitude",
			"latitude", "accuracy", "repost_post_id", "picture_url", "picture_width", "picture_height").
		Values(post.PostID, post.AuthorName, post.Content, post.CreatedAt, post.Longitude,
			post.Latitude, &post.Accuracy, post.RepostPostID, post.PictureURL, post.PictureWidth, post.PictureHeight).ToSql()

	_, err = tx.Exec(ctx, queryString, args...)
	if err != nil {
		ps.logger.Errorw("Error while inserting post", "error", err)
		return nil, status.Error(codes.Internal, "failed to insert post")
	}

	err = ps.insertHashtags(ctx, tx, post)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to insert hashtags")
	}

	if err = ps.db.CommitTx(ctx, tx); err != nil {
		return nil, err
	}

	return &postv1.CreatePostResponse{
		PostId:       post.PostID,
		Author:       users.GetUsers()[0],
		CreationDate: post.CreatedAt.Format(time.RFC3339),
		Content:      post.Content,
		Likes:        0,
		Liked:        false,
		Repost:       repost,
		Location:     request.Location,
		Picture:      picture,
	}, nil
}

func (ps *postService) DeletePost(ctx context.Context, req *postv1.DeletePostRequest) (*postv1.DeletePostResponse, error) {
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Convert post ID to UUID
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		ps.logger.Debugw("Invalid UUID format", "error", err)
		return nil, status.Error(codes.InvalidArgument, "invalid post ID format")
	}

	// Get the post
	posts, err := ps.ListPosts(ctx, &postv1.ListPostsRequest{
		PostIds:    []string{postID.String()},
		Pagination: &commonv1.PaginationRequest{PageSize: 1},
	})
	if err != nil {
		return nil, err
	}

	if len(posts.GetPosts()) == 0 {
		return nil, status.Error(codes.NotFound, "post not found")
	}

	// Check if the authenticated user is the author of the post
	if posts.GetPosts()[0].Author.Username != authenticatedUsername {
		return nil, status.Error(codes.PermissionDenied, "user is not the author")
	}

	conn, err := ps.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := ps.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer ps.db.RollbackTx(ctx, tx)

	// Delete the post
	queryString, args, _ := psql.Delete("posts").Where(sq.Eq{"post_id": postID}).ToSql()
	if _, err = tx.Exec(ctx, queryString, args...); err != nil {
		ps.logger.Errorw("Error in tx.Exec", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete post")
	}

	if err = ps.db.CommitTx(ctx, tx); err != nil {
		return nil, err
	}

	return &postv1.DeletePostResponse{}, nil
}

// ----------------- Helper Functions -----------------

func (ps *postService) retrievePosts(ctx context.Context, conn *pgxpool.Conn, query sq.SelectBuilder, authenticatedUsername string) ([]*postv1.Post, error) {
	queryString, args, err := query.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building SQL query", "error", err)
		return nil, status.Error(codes.Internal, "Failed to build query")
	}

	rows, err := conn.Query(ctx, queryString, args...)
	if err != nil {
		ps.logger.Errorw("Error querying data", "error", err)
		return nil, status.Error(codes.Internal, "Failed to query data")
	}

	posts, err := pgx.CollectRows(rows, pgx.RowToStructByName[schema.Post])
	if err != nil {
		ps.logger.Errorw("Failed to scan rows", "error", err)
		return nil, status.Error(codes.Internal, "Failed to scan rows")
	}

	authorMap, err := ps.getAuthorMap(ctx, posts)
	if err != nil {
		return nil, err
	}

	repostMap, err := ps.getRepostMap(ctx, conn, posts)
	if err != nil {
		return nil, err
	}

	likesMap, err := ps.getLikesMap(ctx, conn, posts)
	if err != nil {
		return nil, err
	}

	likedMap, err := ps.getLikedMap(ctx, conn, posts, authenticatedUsername)
	if err != nil {
		return nil, err
	}

	commentsMap, err := ps.getCommentsMap(ctx, conn, posts)
	if err != nil {
		return nil, err
	}

	protoPosts := make([]*postv1.Post, 0, len(posts))
	for _, post := range posts {
		protoPosts = append(protoPosts, post.ToProto(authorMap, repostMap, likesMap, likedMap, commentsMap))
	}

	return protoPosts, nil
}

func (ps *postService) getAuthorMap(ctx context.Context, posts []schema.Post) (map[string]*userv1.User, error) {
	authorNames := make([]string, 0, len(posts))
	for _, post := range posts {
		authorNames = append(authorNames, post.AuthorName)
	}

	authorsCTX, authorsSpan := ps.tracer.Start(ctx, "Fetch author data")
	authorProfiles, err := ps.profileClient.ListUsers(authorsCTX, &userv1.ListUsersRequest{Usernames: authorNames})
	if err != nil {
		authorsSpan.End()
		ps.logger.Errorw("Error getting user data", "error", err)
		return nil, status.Error(codes.Internal, "Error getting user data")
	}
	authorsSpan.End()

	authorMap := make(map[string]*userv1.User)
	for _, profile := range authorProfiles.GetUsers() {
		authorMap[profile.GetUsername()] = profile
	}

	return authorMap, nil
}

func (ps *postService) getRepostMap(ctx context.Context, conn *pgxpool.Conn, posts []schema.Post) (map[string]*postv1.Post, error) {
	repostIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		if post.RepostPostID != nil {
			repostIDs = append(repostIDs, *post.RepostPostID)
		}
	}

	if len(repostIDs) == 0 {
		return nil, nil
	}

	baseQueryBuilder := basePostQuery.Columns("*").
		Where(sq.Eq{"p.post_id": repostIDs})

	repostQueryString, repostArgs, err := baseQueryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building query", "error", err)
		return nil, status.Error(codes.Internal, "failed to build query")
	}

	rows, err := conn.Query(ctx, repostQueryString, repostArgs...)
	if err != nil {
		ps.logger.Errorw("Error querying data", "error", err)
		return nil, status.Error(codes.Internal, "failed to query data")
	}

	reposts, err := pgx.CollectRows(rows, pgx.RowToStructByName[schema.Post])
	if err != nil {
		ps.logger.Errorw("Error scanning rows", "error", err)
		return nil, status.Error(codes.Internal, "failed to scan rows")
	}
	ps.logger.Infow("Reposts", "reposts", reposts)

	authorMap, err := ps.getAuthorMap(ctx, reposts)
	if err != nil {
		return nil, err
	}

	if len(authorMap) == 0 {
		ps.logger.Warnw("No authors found for reposts", "repostIDs", repostIDs)
	}

	repostMap := make(map[string]*postv1.Post)
	for _, post := range reposts {
		repostMap[post.PostID] = post.ToProto(authorMap, nil, nil, nil, nil)
	}

	return repostMap, nil
}

func (ps *postService) insertHashtags(ctx context.Context, tx pgx.Tx, post *schema.Post) error {
	hashtags := hashtagRegex.FindAllString(post.Content, -1)
	for _, hashtag := range hashtags {
		hashtagId := uuid.New()

		queryString := `INSERT INTO post_service.hashtags (hashtag_id, content) VALUES($1, $2) 
					ON CONFLICT (content) DO UPDATE SET content=hashtags.content 
					RETURNING hashtag_id`
		if err := tx.QueryRow(ctx, queryString, hashtagId, strings.ToLower(hashtag)).Scan(&hashtagId); err != nil {
			ps.logger.Errorw("Error while inserting hashtags", "error", err)
			return err
		}

		queryString = "INSERT INTO post_service.many_posts_has_many_hashtags (post_id_posts, hashtag_id_hashtags) VALUES($1, $2)"
		if _, err := tx.Exec(ctx, queryString, post.PostID, hashtagId); err != nil {
			ps.logger.Errorw("Error while inserting many_posts_has_many_hashtags", "error", err)
			return err
		}
	}
	return nil
}

func (ps *postService) getLikesMap(ctx context.Context, conn *pgxpool.Conn, posts []schema.Post) (map[string]uint32, error) {
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.PostID)
	}

	if len(postIDs) == 0 {
		return nil, nil
	}

	queryBuilder := psql.Select("post_id", "COUNT(*) AS likes").
		From("likes").
		Where(sq.Eq{"post_id": postIDs}).
		GroupBy("post_id")

	queryString, args, err := queryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building query", "error", err)
		return nil, status.Error(codes.Internal, "failed to build query")
	}

	rows, err := conn.Query(ctx, queryString, args...)
	if err != nil {
		ps.logger.Errorw("Error querying data", "error", err)
		return nil, status.Error(codes.Internal, "failed to query data")
	}

	likesMap := make(map[string]uint32)
	for rows.Next() {
		var postID string
		var likes uint32
		if err := rows.Scan(&postID, &likes); err != nil {
			ps.logger.Errorw("Error scanning rows", "error", err)
			return nil, status.Error(codes.Internal, "failed to scan rows")
		}
		likesMap[postID] = likes
	}

	return likesMap, nil
}

func (ps *postService) getLikedMap(ctx context.Context, conn *pgxpool.Conn, posts []schema.Post, authenticatedUsername string) (map[string]bool, error) {
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.PostID)
	}

	if len(postIDs) == 0 {
		return nil, nil
	}

	queryBuilder := psql.Select("post_id").
		From("likes").
		Where(sq.Eq{"post_id": postIDs, "username": authenticatedUsername})

	queryString, args, err := queryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building query", "error", err)
		return nil, status.Error(codes.Internal, "failed to build query")
	}

	rows, err := conn.Query(ctx, queryString, args...)
	if err != nil {
		ps.logger.Errorw("Error querying data", "error", err)
		return nil, status.Error(codes.Internal, "failed to query data")
	}

	likedMap := make(map[string]bool)
	for rows.Next() {
		var postID string
		if err := rows.Scan(&postID); err != nil {
			ps.logger.Errorw("Error scanning rows", "error", err)
			return nil, nil
		}
		likedMap[postID] = true
	}

	return likedMap, nil
}

func (ps *postService) getCommentsMap(ctx context.Context, conn *pgxpool.Conn, posts []schema.Post) (map[string]uint32, error) {
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.PostID)
	}

	if len(postIDs) == 0 {
		return nil, nil
	}

	queryBuilder := psql.Select("post_id", "COUNT(*) AS comments").
		From("comments").
		Where(sq.Eq{"post_id": postIDs}).
		GroupBy("post_id")

	queryString, args, err := queryBuilder.ToSql()
	if err != nil {
		ps.logger.Errorw("Error building query", "error", err)
		return nil, status.Error(codes.Internal, "failed to build query")
	}

	rows, err := conn.Query(ctx, queryString, args...)
	if err != nil {
		ps.logger.Errorw("Error querying data", "error", err)
		return nil, status.Error(codes.Internal, "failed to query data")
	}

	commentsMap := make(map[string]uint32)
	for rows.Next() {
		var postID string
		var comments uint32
		if err := rows.Scan(&postID, &comments); err != nil {
			ps.logger.Errorw("Error scanning rows", "error", err)
			return nil, status.Error(codes.Internal, "failed to scan rows")
		}
		commentsMap[postID] = comments
	}

	return commentsMap, nil
}
