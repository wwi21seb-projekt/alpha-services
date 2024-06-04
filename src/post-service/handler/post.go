package handler

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"regexp"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"time"

	"github.com/wwi21seb-projekt/alpha-shared/db"

	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

var hashtagRegex = regexp.MustCompile(`#\w+`)

type postService struct {
	db            *db.DB
	profileClient pbUser.UserServiceClient
	subscription  pbUser.SubscriptionServiceClient
	pb.UnimplementedPostServiceServer
	pb.UnimplementedFeedServiceServer
}

func NewPostServiceServer(db *db.DB, profileClient pbUser.UserServiceClient, subscription pbUser.SubscriptionServiceClient) pb.PostServiceServer {
	return &postService{
		db:            db,
		profileClient: profileClient,
		subscription:  subscription,
	}
}

func (ps *postService) SearchPosts(ctx context.Context, empty *pb.SearchPostsRequest) (*pb.SearchPostsResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (ps *postService) ListPosts(ctx context.Context, request *pb.ListPostsRequest) (*pb.SearchPostsResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	baseQueryBuilder := psql.Select().
		From("posts p")

	if request.Author != "" {
		baseQueryBuilder = baseQueryBuilder.Where(sq.Eq{"p.author_name": request.Author})
	}

	if request.Hashtag != "" {
		baseQueryBuilder = baseQueryBuilder.Join("many_posts_has_many_hashtags h ON p.post_id = h.post_id_posts").
			Where(sq.Eq{"h.hashtag_id_hashtags": request.Hashtag})
	}

	if request.LikedBy != "" {
		baseQueryBuilder = baseQueryBuilder.LeftJoin("likes l ON p.post_id = l.post_id").
			Where(sq.Eq{"l.username": request.LikedBy})
	}

	if request.CommentedBy != "" {
		baseQueryBuilder = baseQueryBuilder.LeftJoin("comments c ON p.post_id = c.post_id").
			Where(sq.Eq{"c.author_name": request.CommentedBy})
	}

	if request.Pagination.LastPostId != "" {
		baseQueryBuilder = baseQueryBuilder.Where("p.created_at < (SELECT created_at FROM posts WHERE post_id = ?)", request.Pagination.LastPostId)
	}

	// Create the count query
	countQueryBuilder := baseQueryBuilder.
		Columns("COUNT(*)")

	countQueryString, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		log.Errorf("countQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build count query: %v", err)
	}

	var totalRecords int
	err = conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords)
	if err != nil {
		log.Errorf("conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to execute count query: %v", err)
	}

	dataQueryBuilder := baseQueryBuilder.
		Columns("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		OrderBy("p.created_at DESC").
		Limit(uint64(request.Pagination.Limit))

	dataQueryString, dataArgs, err := dataQueryBuilder.ToSql()
	if err != nil {
		log.Errorf("baseQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	log.Infof("dataQueryString: %v", dataQueryString)
	log.Infof("dataArgs: %v", dataArgs)

	log.Infof("countQueryString: %v", countQueryString)
	log.Infof("countArgs: %v", countArgs)

	rows, err := conn.Query(ctx, dataQueryString, dataArgs...)
	if err != nil {
		log.Errorf("conn.Query(ctx, dataQueryString, dataArgs...) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to query data: %v", err)
	}

	var posts []*pb.Post
	for rows.Next() {
		post := &pb.Post{}
		var createdAt time.Time
		var longitude, latitude pgtype.Float8
		var accuracy pgtype.Int4
		var repostId pgtype.Text
		if err = rows.Scan(&post.PostId, &post.Author.Username, &post.Content, &createdAt, &longitude, &latitude, &accuracy, &repostId); err != nil {
			log.Errorf("rows.Scan failed: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to scan row: %v", err)
		}

		if repostId.Valid {
			post.Repost, _, err = ps.retrievePost(ctx, &pb.GetPostRequest{PostId: repostId.String}, conn)
			if err != nil {
				log.Errorf("ps.GetPost failed: %v", err)
				return nil, status.Errorf(codes.Internal, "Failed to get repost: %v", err)
			}
		}

		posts = append(posts, post)
	}

	return nil, nil
}

func (ps *postService) GetPost(ctx context.Context, request *pb.GetPostRequest) (*pb.Post, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	post, repostId, err := ps.retrievePost(ctx, request, conn)
	if err != nil {
		log.Errorf("ps.retrievePost failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to retrieve post: %v", err)
	}

	// Get the repost (one level) if it exists
	if repostId.Valid {
		post.Repost, _, err = ps.retrievePost(ctx, &pb.GetPostRequest{PostId: repostId.String}, conn)
		if err != nil {
			log.Errorf("ps.GetPost failed: %v", err)
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
		log.Errorf("query.ToSql() failed: %v", err)
		return nil, pgtype.Text{}, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	row := conn.QueryRow(ctx, queryString, args...)

	post := &pb.Post{}
	var createdAt time.Time
	var longitude, latitude pgtype.Float8
	var accuracy pgtype.Int4
	var repostId pgtype.Text
	if err = row.Scan(&post.PostId, &post.Author.Username, &post.Content, &createdAt, &longitude, &latitude, &accuracy, &repostId); err != nil {
		log.Errorf("row.Scan failed: %v", err)
		return nil, pgtype.Text{}, status.Errorf(codes.Internal, "Failed to scan row: %v", err)
	}
	return post, repostId, nil
}

func getPostQuery(request *pb.GetPostRequest) sq.SelectBuilder {
	query := psql.Select("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		From("posts p").
		Where(sq.Eq{"p.post_id": request.PostId})

	query = query.Join("many_posts_has_many_hashtags h ON p.post_id = h.post_id_posts")

	return query
}

func (ps *postService) CreatePost(ctx context.Context, request *pb.CreatePostRequest) (*pb.Post, error) {
	// Fetch the username of the authenticated user
	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get Author Profile from User-Service
	user, err := ps.profileClient.GetUser(newCTX, &pbUser.GetUserRequest{Username: authenticatedUsername})
	if err != nil {
		log.Errorf("Error in profileClient.GetUser: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to get author profile: %v", err)
	}

	author := &pbUser.User{
		Username:          user.Username,
		Nickname:          user.Nickname,
		ProfilePictureUrl: user.ProfilePictureUrl,
	}

	post := &pb.Post{
		PostId:       uuid.New().String(),
		CreationDate: time.Now().Format(time.RFC3339),
		Author:       author,
		Content:      request.Content,
		Location:     request.Location,
		Liked:        0,
		Likes:        0,
	}

	log.Debugf("Creating post: %v", post)

	// Start transaction
	tx, err := ps.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer ps.db.Rollback(ctx, tx)

	// if request.RepostedPostID != nil {
	//
	// }

	err = ps.insertPost(ctx, tx, post)
	if err != nil {
		log.Errorf("Error in insertPost: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert post: %v", err)
	}

	err = ps.insertHashtags(ctx, tx, post)
	if err != nil {
		log.Errorf("Error in insertHashtags: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert hashtags: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return post, nil
}

func (ps *postService) DeletePost(ctx context.Context, empty *pb.GetPostRequest) (*pbCommon.Empty, error) {
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

		queryString := `INSERT INTO hashtags (hashtag_id, content) VALUES($1, $2) 
					ON CONFLICT (content) DO UPDATE SET content=hashtags.content 
					RETURNING hashtag_id`
		if err := tx.QueryRow(ctx, queryString, hashtagId, strings.ToLower(hashtag)).Scan(&hashtagId); err != nil {
			return err
		}

		queryString = "INSERT INTO many_posts_has_many_hashtags (post_id_posts, hashtag_id_hashtags) VALUES($1, $2)"
		if _, err := tx.Exec(ctx, queryString, post.PostId, hashtagId); err != nil {
			return err
		}
	}
	return nil
}

func (ps *postService) GetGlobalFeed(ctx context.Context, request *pb.GetFeedRequest) (*pb.SearchPostsResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// baseQueryBuilder := psql.Select().
	// 	Columns("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
	return nil, nil
}
