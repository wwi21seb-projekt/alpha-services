package handler

import (
	"context"
	"errors"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
	"go-micro.dev/v4/logger"
	"google.golang.org/grpc/metadata"
	"regexp"

	"time"

	"github.com/wwi21seb-projekt/alpha-shared/db"

	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

var hashtagRegex = regexp.MustCompile(`#\w+`)

type postService struct {
	db         *db.DB
	UserClient pbUser.UserServiceClient
	pb.UnimplementedPostServiceServer
}

func NewPostServiceServer(db *db.DB, userClient pbUser.UserServiceClient) pb.UserServiceClient {
	return &postService{
		db:            db,
		UserClient: userClient,
	}
}

func (s *postService) QueryPosts(ctx context.Context, empty *pb.Empty) (*pb.QueryPostsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s *postService) DeletePost(ctx context.Context, empty *pb.Empty) (*pb.Empty, error) {
	//TODO implement me
	panic("implement me")
}

func (s *postService) GetFeed(ctx context.Context, request *pb.GetFeedRequest) (*pb.QueryPostsResponse, error) {
	log.Info("GetFeed called")
	response := &pb.QueryPostsResponse{}

	response.Posts = make([]*pb.Post, 0)
	response.Pagination = &pb.PostPagination{
		LastPostId: "",
		Limit:      10,
		Records:    0,
	}

	logger.Info("GetFeed finished, returning: ", response)
	return response, nil
}

func (s *postService) CreatePost(ctx context.Context, request *pb.CreatePostRequest) (*pb.Post, error) {
	// Get the user_id from the context, create a new post_id and get the current time
	userId, ok := ctx.Value("user_id").(string)
	if !ok {
		return nil, errors.New("user_id not found in context")
	}
	postId := uuid.New()
	createdAt := time.Now()

	// Define the transaction function, which inserts the post and hashtags into the database
	txFunc := func(tx pgx.Tx) error {
		if err := s.insertPost(ctx, tx, postId, userId, request.Content, createdAt, request.Location); err != nil {
			return err
		}

		hashtags := hashtagRegex.FindAllString(request.Content, -1)
		if err := s.insertHashtags(ctx, tx, postId, hashtags); err != nil {
			return err
		}

		return nil
	}

	// Execute the transaction
	if err := s.db.Transaction(ctx, txFunc); err != nil {
		return nil, err
	}

	/*// Get the author from the userService outside the transaction to avoid deadlocks
	author, err := s.UserClient.GetAuthor(ctx, &pb.GetAuthorRequest{UserId: userId})
	// Write UserID into Metadata
	md := metadata.Pairs("user_id", userId)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Get the author from the userService outside the transaction to avoid deadlocks
	author, err := s.ProfileClient.GetProfile(ctx, &pb.Empty{})
	if err != nil {
		// Return the error if the author could not be retrieved, but the post was successfully created
		return nil, err
	}*/

	author := &pb.Profile{} //TODO

	// Update rsp fields instead of assigning a new object
	response := &pb.Post{
		PostId:       postId.String(),
		Author:       author,
		Content:      request.Content,
		CreationDate: createdAt.Format(time.RFC3339),
		Location:     request.Location,
	}

	return response, nil
}

func (s *postService) insertPost(ctx context.Context, tx pgx.Tx, postId uuid.UUID, userId, content string, createdAt time.Time, location *pb.Location) error {
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

func (s *postService) insertHashtags(ctx context.Context, tx pgx.Tx, postId uuid.UUID, hashtags []string) error {
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
