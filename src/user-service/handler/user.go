package handler

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type userService struct {
	db *db.DB
	pb.UnimplementedUserServiceServer
}

func NewUserServer(database *db.DB) pb.UserServiceServer {
	return &userService{
		db: database,
	}
}

func (ps userService) GetAuthor(ctx context.Context, request *pb.GetUserRequest) (*pb.User, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	queryBuilder := psql.Select().
		Columns("username", "nickname", "proile_picture_url").
		From("users")

	switch request.UserRequest.(type) {
	case *pb.GetUserRequest_Username:
		queryBuilder = queryBuilder.Where("username = ?", request.GetUsername())
	case *pb.GetUserRequest_UserId:
		queryBuilder = queryBuilder.Where("user_id = ?", request.GetUserId())
	}

	query, args, _ := queryBuilder.ToSql()

	log.Info("Querying user data")
	response := &pb.User{}
	if err = conn.QueryRow(ctx, query, args...).Scan(
		&response.Username, &response.Nickname, &response.ProfilePictureUrl,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Infof("User not found")
			return nil, status.Errorf(codes.NotFound, "User not found")
		}

		log.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}

	return response, nil
}

func (ps userService) GetUser(ctx context.Context, request *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Get authenticated user from metadata
	userId := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	queryBuilder := psql.Select().
		Columns("username", "nickname", "status", "proile_picture_url").
		Column("s1.subscription_id AS subscription_id").
		Column("COUNT(s2.subscription_id) AS following_count").
		Column("COUNT(s3.subscription_id) AS follower_count").
		Column("COUNT(p.post_id) AS post_count").
		From("users u").
		LeftJoin("subscriptions s1 ON s1.subscribee_id = u.user_id AND s1.subscriber_id = ?", userId).
		LeftJoin("subscriptions s2 ON s2.subscriber_id = u.user_id").
		LeftJoin("subscriptions s3 ON s3.subscribee_id = u.user_id").
		LeftJoin("posts p ON p.author_id = u.user_id")

	switch request.UserRequest.(type) {
	case *pb.GetUserRequest_Username:
		queryBuilder = queryBuilder.Where("u.username = ?", request.GetUsername())
	case *pb.GetUserRequest_UserId:
		queryBuilder = queryBuilder.Where("u.user_id = ?", request.GetUserId())
	}

	query, args, _ := queryBuilder.
		GroupBy("u.user_id", "u.username", "u.nickname", "u.email", "u.status", "u.profile_picture_url", "s1.subscription_id").
		ToSql()

	log.Info("Querying user data")
	response := &pb.GetUserResponse{}
	if err = conn.QueryRow(ctx, query, args...).Scan(
		&response.Username, &response.Nickname, &response.Status, &response.ProfilePictureUrl,
		&response.SubscriptionId, &response.FollowingCount, &response.FollowerCount, &response.PostCount,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Infof("User not found")
			return nil, status.Errorf(codes.NotFound, "User not found")
		}

		log.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}

	return response, nil
}

func (ps userService) UpdateUser(ctx context.Context, request *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	tx, err := ps.db.Begin(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Begin failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to begin transaction: %v", err)
	}
	defer ps.db.Rollback(ctx, tx)

	// Get authenticated user from metadata
	userId := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	queryBuilder := psql.Update("users").
		Set("nickname", request.GetNickname()).
		Set("status", request.GetStatus()).
		Where("user_id = ?", userId)

	query, args, _ := queryBuilder.ToSql()

	log.Info("Updating user data")
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Commit: %v", err)
	}

	return &pb.UpdateUserResponse{
		Nickname: request.GetNickname(),
		Status:   request.GetStatus(),
	}, nil
}

func (ps userService) SearchUsers(ctx context.Context, request *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	dataQuery, dataArgs, _ := psql.Select().
		Columns("username", "nickname", "profile_picture_url").
		Column("levenstein(username, ?) AS distance", request.GetQuery()).
		From("users").
		Where("levenstein(u.username, ?) <= 5", request.GetQuery()).
		OrderBy("distance").
		Limit(uint64(request.GetPagination().GetLimit())).
		Offset(uint64(request.GetPagination().GetOffset())).
		ToSql()

	countQuery, countArgs, _ := psql.Select("COUNT(*)").
		From("users").
		Where("levenstein(username, ?) <= 5", request.GetQuery()).
		ToSql()

	log.Info("Querying user data")
	rows, err := conn.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		log.Errorf("Error in conn.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.Query: %v", err)
	}

	users, err := scanUsers(rows)
	if err != nil {
		log.Errorf("Error in scanUsers: %v", err)
		return nil, err
	}

	// Get total count
	records := 0
	if err = conn.QueryRow(ctx, countQuery, countArgs...).Scan(&records); err != nil {
		log.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}

	return &pb.SearchUsersResponse{
		Profiles: users,
		Pagination: &pbCommon.Pagination{
			Records: int32(records),
			Offset:  request.GetPagination().GetOffset(),
			Limit:   request.GetPagination().GetLimit(),
		},
	}, nil
}

func (ps userService) ListUsers(ctx context.Context, request *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}

	queryBuilder, queryArgs, _ := psql.Select().
		Columns("username", "nickname", "profile_picture_url").
		From("users").
		Where("user_id IN (?)", request.GetUserIds()).
		ToSql()

	log.Info("Querying user data")
	rows, err := conn.Query(ctx, queryBuilder, queryArgs...)
	if err != nil {
		log.Errorf("Error in conn.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.Query: %v", err)
	}

	users, err := scanUsers(rows)
	if err != nil {
		log.Errorf("Error in scanUsers: %v", err)
		return nil, err
	}

	return &pb.ListUsersResponse{
		Profiles: users,
	}, nil
}

func scanUsers(rows pgx.Rows) ([]*pb.User, error) {
	var users []*pb.User
	for rows.Next() {
		user := &pb.User{}
		if err := rows.Scan(&user.Username, &user.Nickname, &user.ProfilePictureUrl); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}
