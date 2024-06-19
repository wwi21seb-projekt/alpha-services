package handler

import (
	"context"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"time"
)

// var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type feedService struct {
	logger             *zap.SugaredLogger
	tracer             trace.Tracer
	db                 *db.DB
	profileClient      pbUser.UserServiceClient
	subscriptionClient pbUser.SubscriptionServiceClient
	pb.UnimplementedFeedServiceServer
}

func NewFeedServiceServer(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient, subClient pbUser.SubscriptionServiceClient) pb.FeedServiceServer {
	return &feedService{
		logger:             logger,
		tracer:             otel.GetTracerProvider().Tracer("post-service"),
		db:                 db,
		profileClient:      profileClient,
		subscriptionClient: subClient,
	}
}

func (fs *feedService) GetPersonalFeed(ctx context.Context, request *pb.GetFeedRequest) (*pb.SearchPostsResponse, error) {
	conn, err := fs.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	authenticatedUsername := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	newCTX := metadata.AppendToOutgoingContext(ctx, string(keys.SubjectKey), authenticatedUsername)

	// Get the usernames of all people I follow
	subCtx, subSpan := fs.tracer.Start(newCTX, "GetSubscriptions")
	subscriptions, err := fs.subscriptionClient.ListSubscriptions(subCtx, &pbUser.ListSubscriptionsRequest{Username: authenticatedUsername, SubscriptionType: pbUser.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING, Pagination: &pbCommon.PaginationRequest{Limit: 1000}})
	if err != nil {
		subSpan.End()
		fs.logger.Errorf("Error in subscriptionClient.ListSubscriptions: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to get subscriptions: %v", err)
	}
	subSpan.End()

	fs.logger.Infof("Subscriptions: %v", subscriptions.Subscriptions)

	peopleIFollow := make([]string, 0)
	for _, sub := range subscriptions.Subscriptions {
		peopleIFollow = append(peopleIFollow, sub.Username)
	}

	// Include the authenticated user in the feed
	peopleIFollow = append(peopleIFollow, authenticatedUsername)

	baseQueryBuilder := psql.Select().
		From("posts p").
		LeftJoin("likes l ON p.post_id = l.post_id").
		LeftJoin("comments c ON p.post_id = c.post_id")

	authoredCondition := sq.Eq{"p.author_name": peopleIFollow}
	likedCondition := sq.Eq{"l.username": peopleIFollow}
	commentedCondition := sq.Eq{"c.author_name": peopleIFollow}

	baseQueryBuilder = baseQueryBuilder.Where(sq.Or{authoredCondition, likedCondition, commentedCondition})

	if request.LastPostId != "" {
		baseQueryBuilder = baseQueryBuilder.Where("p.created_at < (SELECT created_at FROM posts WHERE post_id = ?)", request.LastPostId)
	}

	countQueryBuilder := baseQueryBuilder.
		Columns("COUNT(p.post_id)")

	countQueryString, countArgs, err := countQueryBuilder.ToSql()
	if err != nil {
		fs.logger.Errorf("countQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build count query: %v", err)
	}

	var totalRecords int
	err = conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords)
	if err != nil {
		fs.logger.Errorf("conn.QueryRow(ctx, countQueryString, countArgs...).Scan(&totalRecords) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to execute count query: %v", err)
	}

	dataQueryBuilder := baseQueryBuilder.
		Columns("p.post_id", "p.author_name", "p.content", "p.created_at", "p.longitude", "p.latitude", "p.accuracy", "p.repost_post_id").
		OrderBy("p.created_at DESC").
		Limit(uint64(request.Limit))

	dataQueryString, dataArgs, err := dataQueryBuilder.ToSql()
	if err != nil {
		fs.logger.Errorf("baseQueryBuilder.ToSql() failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	dbCTX, dbSpan := fs.tracer.Start(ctx, "QueryPosts")
	dbSpan.SetAttributes(attribute.String("db.statement", dataQueryString))
	rows, err := conn.Query(dbCTX, dataQueryString, dataArgs...)
	if err != nil {
		dbSpan.End()
		fs.logger.Errorf("conn.Query(ctx, dataQueryString, dataArgs...) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to query data: %v", err)
	}
	dbSpan.End()

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
			fs.logger.Errorf("rows.Scan failed: %v", err)
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
	err = fs.populateReposts(ctx, conn, posts, repostIDs)
	if err != nil {
		return nil, err
	}

	resp := &pb.SearchPostsResponse{
		Posts: posts,
		Pagination: &pbCommon.Pagination{
			Limit:   request.Limit,
			Records: int32(totalRecords),
		},
	}

	return resp, nil
}

func (fs *feedService) populateReposts(ctx context.Context, conn *pgxpool.Conn, posts []*pb.Post, repostIDs map[string]string) error {
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
		fs.logger.Errorf("repostQueryBuilder.ToSql() failed: %v", err)
		return status.Errorf(codes.Internal, "Failed to build query: %v", err)
	}

	rows, err := conn.Query(ctx, repostQueryString, repostArgs...)
	if err != nil {
		fs.logger.Errorf("conn.Query(ctx, repostQueryString, repostArgs...) failed: %v", err)
		return status.Errorf(codes.Internal, "Failed to query data: %v", err)
	}

	reposts := make(map[string]*pb.Post) // Map of repost_post_id -> repost
	for rows.Next() {
		repost := &pb.Post{Author: &pbUser.PublicUser{}}
		location := &pb.Location{}
		var creationDate pgtype.Timestamptz
		if err = rows.Scan(&repost.PostId, &repost.Author.Username, &repost.Content, &creationDate, &location.Longitude, &location.Latitude, &location.Accuracy); err != nil {
			fs.logger.Errorf("rows.Scan failed: %v", err)
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
