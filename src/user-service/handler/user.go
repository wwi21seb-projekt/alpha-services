package handler

import (
	"context"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type userService struct {
	logger *zap.SugaredLogger
	tracer trace.Tracer
	db     *db.DB
	pb.UnimplementedUserServiceServer
}

func NewUserServer(logger *zap.SugaredLogger, database *db.DB) pb.UserServiceServer {
	return &userService{
		logger: logger,
		tracer: otel.GetTracerProvider().Tracer("user-service"),
		db:     database,
	}
}

func (us userService) GetUser(ctx context.Context, request *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	conn, err := us.db.Pool.Acquire(ctx)
	if err != nil {
		us.logger.Errorf("us.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()
	// Get authenticated user from metadata
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Select user data
	selectCtx, selectSpan := us.tracer.Start(ctx, "SelectUserData")
	query, args, _ := psql.Select().
		Columns("u.nickname", "u.status", "u.picture_url", "u.picture_width", "u.picture_height").
		Column("s1.subscription_id AS subscription_id").
		Column("COUNT(s2.subscription_id) AS following_count").
		Column("COUNT(s3.subscription_id) AS follower_count").
		//Column("COUNT(p.post_id) AS post_count").
		From("users u").
		LeftJoin("subscriptions s1 ON s1.subscribee_name = u.username AND s1.subscriber_name = ?", username).
		LeftJoin("subscriptions s2 ON s2.subscriber_name = u.username").
		LeftJoin("subscriptions s3 ON s3.subscribee_name = u.username").
		//LeftJoin("posts p ON p.author_name = u.username").
		Where("u.username = ?", request.GetUsername()).
		GroupBy("u.nickname", "u.status", "u.profile_picture_url", "s1.subscription_id").
		ToSql()

	var nickname, userStatus, pictureUrl, subscriptionID pgtype.Text
	var followingCount, followerCount, pictureWidth, pictureHeight pgtype.Int4

	us.logger.Info("Querying user data")
	if err = conn.QueryRow(selectCtx, query, args...).Scan(
		&nickname, &userStatus, &pictureUrl, &pictureWidth,
		&pictureHeight, &subscriptionID, &followingCount, &followerCount,
		//&response.PostCount,
	); err != nil {
		selectSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			us.logger.Infof("User not found")
			return nil, status.Errorf(codes.NotFound, "User not found")
		}

		us.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}
	selectSpan.End()

	response := &pb.GetUserResponse{
		Username:       request.Username,
		Nickname:       nickname.String,
		Status:         userStatus.String,
		SubscriptionId: subscriptionID.String,
		FollowingCount: followingCount.Int32,
		FollowerCount:  followerCount.Int32,
		PostCount:      -1,
	}

	if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
		response.Picture = &pbCommon.Picture{
			Url:    pictureUrl.String,
			Width:  pictureWidth.Int32,
			Height: pictureHeight.Int32,
		}
	}

	return response, nil
}

func (us userService) UpdateUser(ctx context.Context, request *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	tx, err := us.db.Begin(ctx)
	if err != nil {
		us.logger.Errorf("us.db.Pool.Begin failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to begin transaction: %v", err)
	}
	defer us.db.Rollback(ctx, tx)

	// Get authenticated user from metadata
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Update user data
	updateCtx, updateSpan := us.tracer.Start(ctx, "UpdateUserData")
	query, args, _ := psql.Update("users").
		Set("nickname", request.GetNickname()).
		Set("status", request.GetStatus()).
		Where("username = ?", username).
		ToSql()

	us.logger.Info("Updating user data")
	if _, err = tx.Exec(updateCtx, query, args...); err != nil {
		updateSpan.End()
		us.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}
	updateSpan.End()

	if err = us.db.Commit(ctx, tx); err != nil {
		us.logger.Errorf("Error in us.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in us.db.Commit: %v", err)
	}

	return &pb.UpdateUserResponse{
		Nickname: request.GetNickname(),
		Status:   request.GetStatus(),
	}, nil
}

func (us userService) SearchUsers(ctx context.Context, request *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	conn, err := us.db.Pool.Acquire(ctx)
	if err != nil {
		us.logger.Errorf("us.db.Pool.Acquire failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Select user data
	selectCtx, selectSpan := us.tracer.Start(ctx, "SelectUserData")
	dataQuery, dataArgs, _ := psql.Select().
		Columns("username", "nickname", "picture_url", "picture_width", "picture_height").
		Column("levenshtein(username, ?) AS distance", request.GetQuery()).
		From("users").
		Where("levenshtein(username, ?) <= 5", request.GetQuery()).
		OrderBy("distance").
		Limit(uint64(request.GetPagination().GetLimit())).
		Offset(uint64(request.GetPagination().GetOffset())).
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
	users := make([]*pb.PublicUser, 0)
	levenshteinDistance := 0
	for rows.Next() {
		user := &pb.PublicUser{}
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
			user.Picture = &pbCommon.Picture{
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

	return &pb.SearchUsersResponse{
		Users: users,
		Pagination: &pbCommon.Pagination{
			Records: int32(records),
			Offset:  request.GetPagination().GetOffset(),
			Limit:   request.GetPagination().GetLimit(),
		},
	}, nil
}

func (us userService) ListUsers(ctx context.Context, request *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	conn, err := us.db.Pool.Acquire(ctx)
	if err != nil {
		us.logger.Errorf("us.db.Pool.Acquire failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
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

	return &pb.ListUsersResponse{
		Users: users,
	}, nil
}

func scanUsers(rows pgx.Rows) ([]*pb.PublicUser, error) {
	var users []*pb.PublicUser
	for rows.Next() {
		user := &pb.PublicUser{}
		var nickname, pictureUrl pgtype.Text
		var pictureWidth, pictureHeight pgtype.Int4

		if err := rows.Scan(&user.Username, &nickname, &pictureUrl, &pictureWidth, &pictureHeight); err != nil {
			return nil, err
		}

		if nickname.Valid {
			user.Nickname = nickname.String
		}
		if pictureUrl.Valid && pictureWidth.Valid && pictureHeight.Valid {
			user.Picture = &pbCommon.Picture{
				Url:    pictureUrl.String,
				Width:  pictureWidth.Int32,
				Height: pictureHeight.Int32,
			}
		}

		users = append(users, user)
	}
	return users, nil
}
