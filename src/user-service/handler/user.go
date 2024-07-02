package handler

import (
	"context"
	"errors"
	"fmt"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	postv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/post/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
	"os"
	"strconv"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type userService struct {
	logger      *zap.SugaredLogger
	tracer      trace.Tracer
	db          *db.DB
	postClient  postv1.PostServiceClient
	imageClient imagev1.ImageServiceClient
	userv1.UnimplementedUserServiceServer
}

func NewUserServer(logger *zap.SugaredLogger, database *db.DB, postClient postv1.PostServiceClient, imageClient imagev1.ImageServiceClient) userv1.UserServiceServer {
	return &userService{
		logger:      logger,
		tracer:      otel.GetTracerProvider().Tracer("user-service"),
		db:          database,
		postClient:  postClient,
		imageClient: imageClient,
	}
}

func (us userService) GetUser(ctx context.Context, request *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	conn, err := us.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	// Get authenticated user from metadata
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Check if user is activated
	activated, err := us.isUserActivated(ctx, username)
	if err != nil {
		us.logger.Errorf("Error in us.db.IsUserActivated: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in us.db.IsUserActivated: %v", err)
	}
	if !activated {
		us.logger.Infof("User not activated")
		return nil, status.Errorf(codes.PermissionDenied, "User not activated")
	}

	// Select user data
	queryBuilder := psql.Select().
		Columns("u.nickname", "u.status", "u.picture_url", "u.picture_width", "u.picture_height").
		Column("s1.subscription_id AS subscription_id").
		Column("(SELECT COUNT(DISTINCT s2.subscription_id) FROM subscriptions s2 WHERE s2.subscriber_name = u.username) AS following_count").
		Column("(SELECT COUNT(DISTINCT s3.subscription_id) FROM subscriptions s3 WHERE s3.subscribee_name = u.username) AS follower_count").
		From("users u").
		LeftJoin("subscriptions s1 ON s1.subscribee_name = u.username AND s1.subscriber_name = ?", username).
		Where("u.username = ?", request.GetUsername())

	// Generate the SQL query from the query builder
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		us.logger.Errorf("Error building SQL query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error building SQL query: %v", err)
	}

	var nickname, userStatus, pictureUrl, subscriptionID pgtype.Text
	var followingCount, followerCount, pictureWidth, pictureHeight pgtype.Int4

	if err = conn.QueryRow(ctx, query, args...).Scan(
		&nickname, &userStatus, &pictureUrl, &pictureWidth,
		&pictureHeight, &subscriptionID, &followingCount, &followerCount,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			us.logger.Infof("User not found")
			return nil, status.Errorf(codes.NotFound, "User not found")
		}

		us.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}

	// Get post count by pagination response of ListPosts
	postCount, err := us.getPostCount(ctx, request.GetUsername())
	if err != nil {
		return nil, err
	}

	response := &userv1.GetUserResponse{
		Username:       request.Username,
		Nickname:       nickname.String,
		Status:         userStatus.String,
		SubscriptionId: subscriptionID.String,
		FollowingCount: followingCount.Int32,
		FollowerCount:  followerCount.Int32,
		PostCount:      postCount,
	}

	if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
		response.Picture = &imagev1.Picture{
			Url:    pictureUrl.String,
			Width:  pictureWidth.Int32,
			Height: pictureHeight.Int32,
		}
	}

	return response, nil
}

func (us userService) getPostCount(ctx context.Context, username string) (int32, error) {
	resp, err := us.postClient.ListPosts(ctx, &postv1.ListPostsRequest{
		Username:   &username,
		FeedType:   postv1.FeedType_FEED_TYPE_USER,
		Pagination: &commonv1.PaginationRequest{PageSize: 0},
	})
	if err != nil {
		us.logger.Errorf("Error in postClient.ListPosts: %v", err)
		return -1, status.Errorf(codes.Internal, "Error in postClient.ListPosts: %v", err)
	}

	return resp.GetPagination().GetTotalSize(), nil
}

func (us userService) UpdateUser(ctx context.Context, request *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	queryBuilder := psql.Update("users").Where(sq.Eq{"username": username})

	if request.GetNickname() != "" {
		queryBuilder = queryBuilder.Set("nickname", request.GetNickname())
	}

	if request.GetStatus() != "" {
		queryBuilder = queryBuilder.Set("status", request.GetStatus())
	}

	// Upload image to storage if provided
	imageUrl := defaultAvatarURL
	imageWidth := defaultAvatarWidth
	imageHeight := defaultAvatarHeight

	// Before we start the transaction, we need to upload the picture to the storage service
	if request.GetBase64Picture() != "" {
		uploadImageCtx, uploadImageSpan := us.tracer.Start(ctx, "UploadImage")
		us.logger.Debugw("Uploading image to storage...", "username", username)
		uploadImageResponse, err := us.imageClient.UploadImage(uploadImageCtx, &imagev1.UploadImageRequest{
			Image: request.GetBase64Picture(),
			Name:  username,
		})
		if err != nil {
			us.logger.Errorf("Error in upstream call imageClient.UploadImage: %v", err)
			uploadImageSpan.End()
			return nil, status.Errorf(codes.Internal, "failed to upload image: %v", err)
		}

		environment := os.Getenv("ENVIRONMENT")
		if environment == "local" {
			imageUrl = fmt.Sprintf("http://localhost:8080/api/images?image=%s", uploadImageResponse.GetUrl())
		} else {
			imageUrl = fmt.Sprintf("https://alpha.c930.net/api/images?image=%s", uploadImageResponse.GetUrl())
		}

		imageWidth = 500  // replace with actual width
		imageHeight = 500 // replace with actual height

		uploadImageSpan.End()
	}

	if request.Base64Picture != nil {
		queryBuilder = queryBuilder.Set("picture_url", imageUrl).
			Set("picture_width", imageWidth).
			Set("picture_height", imageHeight)
	}

	// Update user data
	query, args, _ := queryBuilder.ToSql()

	conn, err := us.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := us.db.BeginTx(ctx, conn)
	if err != nil {
		us.logger.Errorf("us.db.Pool.Begin failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to begin transaction: %v", err)
	}
	defer us.db.RollbackTx(ctx, tx)

	us.logger.Info("Updating user data")
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		us.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	if err = us.db.CommitTx(ctx, tx); err != nil {
		us.logger.Errorf("Error in us.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in us.db.Commit: %v", err)
	}

	return &userv1.UpdateUserResponse{
		Nickname: request.GetNickname(),
		Status:   request.GetStatus(),
	}, nil
}

func (us userService) SearchUsers(ctx context.Context, request *userv1.SearchUsersRequest) (*userv1.SearchUsersResponse, error) {
	conn, err := us.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	offset, err := strconv.Atoi(request.GetPagination().GetPageToken())
	if err != nil {
		offset = 0
	}

	// Select user data
	selectCtx, selectSpan := us.tracer.Start(ctx, "SelectUserData")
	dataQuery, dataArgs, _ := psql.Select().
		Columns("username", "nickname", "picture_url", "picture_width", "picture_height").
		Column("levenshtein(username, ?) AS distance", request.GetQuery()).
		From("users").
		Where("levenshtein(username, ?) <= 5", request.GetQuery()).
		OrderBy("distance").
		Limit(uint64(request.GetPagination().GetPageSize())).
		Offset(uint64(offset)).
		ToSql()

	countQuery, countArgs, _ := psql.Select("COUNT(*)").
		From("users").
		Where("levenshtein(username, ?) <= 5", request.GetQuery()).
		ToSql()

	// Start batch request
	batch := &pgx.Batch{}
	batch.Queue(dataQuery, dataArgs...)
	batch.Queue(countQuery, countArgs...)
	us.logger.Info("Starting batch request for user data")
	br := conn.SendBatch(selectCtx, batch)

	// Get data from batch request
	rows, err := br.Query()
	if err != nil {
		selectSpan.End()
		us.logger.Errorf("Error in conn.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.Query: %v", err)
	}
	selectSpan.End()

	// Scan users
	_, scanUserSpan := us.tracer.Start(ctx, "ScanUsersFromRows")
	users := make([]*userv1.User, 0)
	levenshteinDistance := 0
	for rows.Next() {
		user := &userv1.User{}
		var nickname, pictureUrl pgtype.Text
		var pictureWidth, pictureHeight pgtype.Int4

		if err = rows.Scan(&user.Username, &nickname, &pictureUrl, &pictureWidth, &pictureHeight, &levenshteinDistance); err != nil {
			scanUserSpan.End()
			us.logger.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Errorf(codes.Internal, "Error in rows.Scan: %v", err)
		}

		if nickname.Valid {
			user.Nickname = nickname.String
		}
		if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
			user.Picture = &imagev1.Picture{
				Url:    pictureUrl.String,
				Width:  pictureWidth.Int32,
				Height: pictureHeight.Int32,
			}
		}

		users = append(users, user)
	}
	scanUserSpan.End()

	// Get total count
	_, scanRecordSpan := us.tracer.Start(ctx, "ScanRecordsFromRows")
	records := 0
	if err = br.QueryRow().Scan(&records); err != nil {
		scanRecordSpan.End()
		us.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}
	scanRecordSpan.End()

	return &userv1.SearchUsersResponse{
		Users: users,
		Pagination: &commonv1.PaginationResponse{
			TotalSize:     int32(records),
			NextPageToken: request.GetPagination().GetPageToken(),
		},
	}, nil
}

func (us userService) ListUsers(ctx context.Context, request *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	conn, err := us.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	// Select user data
	selectCtx, selectSpan := us.tracer.Start(ctx, "SelectUserData")
	queryBuilder, queryArgs, _ := psql.Select().
		Columns("username", "nickname", "picture_url", "picture_width", "picture_height").
		From("users").
		Where(sq.Eq{"username": request.GetUsernames()}).
		ToSql()

	us.logger.Info("Querying user data")
	rows, err := conn.Query(selectCtx, queryBuilder, queryArgs...)
	if err != nil {
		selectSpan.End()
		us.logger.Errorf("Error in conn.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.Query: %v", err)
	}
	selectSpan.End()

	_, scanSpan := us.tracer.Start(ctx, "ScanUsers")
	defer scanSpan.End() // defer here since we'll return after this
	users, err := scanUsers(rows)
	if err != nil {
		us.logger.Errorf("Error in scanUsers: %v", err)
		return nil, err
	}

	return &userv1.ListUsersResponse{
		Users: users,
	}, nil
}

func (us userService) isUserActivated(ctx context.Context, username string) (bool, error) {
	conn, err := us.db.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()

	// Check if user is activated
	selectCtx, selectSpan := us.tracer.Start(ctx, "SelectUserStatus")
	query, args, _ := psql.Select("activated_at").
		From("users").
		Where("username = ?", username).
		ToSql()

	var activatedAt pgtype.Timestamptz
	if err = conn.QueryRow(selectCtx, query, args...).Scan(&activatedAt); err != nil {
		selectSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			us.logger.Infof("User not found")
			return false, nil
		}

		us.logger.Errorf("Error in conn.QueryRow: %v", err)
		return false, err
	}
	selectSpan.End()

	return activatedAt.Valid, nil
}

func scanUsers(rows pgx.Rows) ([]*userv1.User, error) {
	var users []*userv1.User
	for rows.Next() {
		user := &userv1.User{}
		var nickname, pictureUrl pgtype.Text
		var pictureWidth, pictureHeight pgtype.Int4

		if err := rows.Scan(&user.Username, &nickname, &pictureUrl, &pictureWidth, &pictureHeight); err != nil {
			return nil, err
		}

		if nickname.Valid {
			user.Nickname = nickname.String
		}
		if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
			user.Picture = &imagev1.Picture{
				Url:    pictureUrl.String,
				Width:  pictureWidth.Int32,
				Height: pictureHeight.Int32,
			}
		}

		users = append(users, user)
	}
	return users, nil
}
