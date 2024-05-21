package handler

import (
	"context"
	"errors"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"regexp"

	"github.com/wwi21seb-projekt/alpha-services/src/shared/db"
	"time"

	pb "github.com/wwi21seb-projekt/alpha-services/src/post-service/proto"
)

type PostService struct {
	db          *db.DB
	UserService pb.UserService
}

func (s *PostService) QueryPosts(ctx context.Context, empty *pb.Empty, response *pb.QueryPostsResponse) error {
	//TODO implement me
	panic("implement me")
}

func (s *PostService) DeletePost(ctx context.Context, empty *pb.Empty, empty2 *pb.Empty) error {
	//TODO implement me
	panic("implement me")
}

func (s *PostService) GetFeed(ctx context.Context, request *pb.GetFeedRequest, response *pb.QueryPostsResponse) error {
	//TODO implement me
	panic("implement me")
}

func NewPostService(db *db.DB, userService pb.UserService) *PostService {
	return &PostService{
		db:          db,
		UserService: userService,
	}
}

var hashtagRegex = regexp.MustCompile(`#\w+`)

func (s *PostService) CreatePost(ctx context.Context, req *pb.CreatePostRequest, rsp *pb.Post) error {
	// Get the user_id from the context, create a new post_id and get the current time
	userId, ok := ctx.Value("user_id").(string)
	if !ok {
		return errors.New("user_id not found in context")
	}
	postId := uuid.New()
	createdAt := time.Now()

	// Define the transaction function, which inserts the post and hashtags into the database
	txFunc := func(tx pgx.Tx) error {
		if err := s.insertPost(ctx, tx, postId, userId, req.Content, createdAt, req.Location); err != nil {
			return err
		}

		hashtags := hashtagRegex.FindAllString(req.Content, -1)
		if err := s.insertHashtags(ctx, tx, postId, hashtags); err != nil {
			return err
		}

		return nil
	}

	// Execute the transaction
	if err := s.db.Transaction(ctx, txFunc); err != nil {
		return err
	}

	// Get the author from the userService outside the transaction to avoid deadlocks
	author, err := s.UserService.GetAuthor(ctx, &pb.GetAuthorRequest{UserId: userId})
	if err != nil {
		// Return the error if the author could not be retrieved, but the post was successfully created
		return err
	}

	// Update rsp fields instead of assigning a new object
	rsp.PostId = postId.String()
	rsp.Author = author
	rsp.Content = req.Content
	rsp.CreationDate = createdAt.Format(time.RFC3339)
	rsp.Location = req.Location

	return nil
}

func (s *PostService) insertPost(ctx context.Context, tx pgx.Tx, postId uuid.UUID, userId, content string, createdAt time.Time, location *pb.Location) error {
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

func (s *PostService) insertHashtags(ctx context.Context, tx pgx.Tx, postId uuid.UUID, hashtags []string) error {
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
