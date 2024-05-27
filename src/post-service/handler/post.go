package handler

import (
	"context"
	"regexp"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"time"

	"github.com/wwi21seb-projekt/alpha-shared/db"

	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

var hashtagRegex = regexp.MustCompile(`#\w+`)

type postService struct {
	db            *db.DB
	profileClient pbUser.UserServiceClient
	subscription  pbUser.SubscriptionServiceClient
	pb.UnimplementedPostServiceServer
}

func NewPostServiceServer(db *db.DB, profileClient pbUser.UserServiceClient, subscription pbUser.SubscriptionServiceClient) pb.PostServiceServer {
	return &postService{
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
	//TODO implement me
	panic("implement me")
}

func (ps *postService) GetPost(ctx context.Context, request *pb.GetPostRequest) (*pb.Post, error) {
	panic("implement me")
}

func (ps *postService) CreatePost(ctx context.Context, request *pb.CreatePostRequest) (*pb.Post, error) {
	panic("implement me")
}

func (ps *postService) DeletePost(ctx context.Context, empty *pb.GetPostRequest) (*pbCommon.Empty, error) {
	panic("implement me")
}

func (ps *postService) insertPost(ctx context.Context, tx pgx.Tx, postId uuid.UUID, userId, content string, createdAt time.Time, location *pb.Location) error {
	// Start building the query
	query := squirrel.Insert("alpha_schema.posts").
		Columns("post_id", "author_id", "content", "created_at").
		Values(postId, userId, content, createdAt)

	// If location is provided, add location data to the query
	if location != nil {
		query = query.Columns("longitude", "latitude", "accuracy").
			Values(location.Longitude, location.Latitude, location.Accuracy)
	}

	// Finalize and execute the query
	queryString, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, queryString, args...)
	return err
}

func (ps *postService) insertHashtags(ctx context.Context, tx pgx.Tx, postId uuid.UUID, hashtags []string) error {
	for _, hashtag := range hashtags {
		hashtagId := uuid.New()

		queryString := `INSERT INTO alpha_schema.hashtags (hashtag_id, content) VALUES($1, $2) 
					ON CONFLICT (content) DO UPDATE SET content=alpha_schema.hashtags.content 
					RETURNING hashtag_id`
		if err := tx.QueryRow(ctx, queryString, hashtagId, hashtag).Scan(&hashtagId); err != nil {
			return err
		}

		queryString = "INSERT INTO alpha_schema.many_posts_has_many_hashtags (post_id_posts, hashtag_id_hashtags) VALUES($1, $2)"
		if _, err := tx.Exec(ctx, queryString, postId, hashtagId); err != nil {
			return err
		}
	}
	return nil
}
