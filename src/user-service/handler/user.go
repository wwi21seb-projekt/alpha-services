package handler

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

func (ps userService) GetUser(ctx context.Context, request *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	conn, err := ps.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("ps.db.Pool.Acquire(ctx) failed: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Get authenticated user from metadata
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	query, args, _ := psql.Select().
		Columns("u.nickname", "u.status", "u.profile_picture_url").
		Column("s1.subscription_id AS subscription_id").
		Column("COUNT(s2.subscription_id) AS following_count").
		Column("COUNT(s3.subscription_id) AS follower_count").
		Column("COUNT(p.post_id) AS post_count").
		From("users u").
		LeftJoin("subscriptions s1 ON s1.subscribee_name = u.username AND s1.subscriber_name = ?", username).
		LeftJoin("subscriptions s2 ON s2.subscriber_name = u.username").
		LeftJoin("subscriptions s3 ON s3.subscribee_name = u.username").
		LeftJoin("posts p ON p.author_name = u.username").
		Where("u.username = ?", request.GetUsername()).
		GroupBy("u.nickname", "u.status", "u.profile_picture_url", "s1.subscription_id").
		ToSql()

	log.Info("Querying user data")
	response := &pb.GetUserResponse{}
	if err = conn.QueryRow(ctx, query, args...).Scan(
		&response.Nickname, &response.Status, &response.ProfilePictureUrl, &response.SubscriptionId,
		&response.FollowingCount, &response.FollowerCount, &response.PostCount,
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
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	query, args, _ := psql.Update("users").
		Set("nickname", request.GetNickname()).
		Set("status", request.GetStatus()).
		Where("username = ?", username).
		ToSql()

	log.Info("Updating user data")
	log.Infof("Query: %s", query)
	log.Infof("Args: %v", args)
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in tx.Exec: %v", err)
	}

	if err = ps.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in ps.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in ps.db.Commit: %v", err)
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
	log.Info("Starting batch request for user data")
	br := conn.SendBatch(ctx, batch)

	// Get data from batch request
	rows, err := br.Query()
	if err != nil {
		log.Errorf("Error in conn.Query: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.Query: %v", err)
	}

	users := make([]*pb.User, 0)
	levenshteinDistance := 0
	for rows.Next() {
		user := &pb.User{}
		var nickname, profilePictureUrl = pgtype.Text{}, pgtype.Text{}

		if err = rows.Scan(&user.Username, &nickname, &profilePictureUrl, &levenshteinDistance); err != nil {
			log.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Errorf(codes.Internal, "Error in rows.Scan: %v", err)
		}
		if nickname.Valid {
			user.Nickname = nickname.String
		}
		if profilePictureUrl.Valid {
			user.ProfilePictureUrl = profilePictureUrl.String
		}

		users = append(users, user)
	}

	// Get total count
	records := 0
	if err = br.QueryRow().Scan(&records); err != nil {
		log.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "Error in conn.QueryRow: %v", err)
	}

	return &pb.SearchUsersResponse{
		Users: users,
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
		Where("user_id IN (?)", request.GetUsernames()).
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
		Users: users,
	}, nil
}

func scanUsers(rows pgx.Rows) ([]*pb.User, error) {
	var users []*pb.User
	for rows.Next() {
		user := &pb.User{}
		var nickname, profilePictureUrl = pgtype.Text{}, pgtype.Text{}
		if err := rows.Scan(&user.Username, &nickname, &profilePictureUrl); err != nil {
			return nil, err
		}
		if nickname.Valid {
			user.Nickname = nickname.String
		}
		if profilePictureUrl.Valid {
			user.ProfilePictureUrl = profilePictureUrl.String
		}
		users = append(users, user)
	}
	return users, nil
}
