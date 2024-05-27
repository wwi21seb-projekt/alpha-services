package handler

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	common "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type authenticationService struct {
	db *db.DB
	pb.UnimplementedAuthenticationServiceServer
}

func NewAuthenticationServer(database *db.DB) pb.AuthenticationServiceServer {
	return &authenticationService{
		db: database,
	}
}

func (as authenticationService) RegisterUser(ctx context.Context, request *pb.RegisterUserRequest) (*pb.User, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		return nil, status.Errorf(codes.Unknown, "failed to hash password: %v", err)
	}

	userId := uuid.New()
	createdAt := time.Now()
	expiresAt := createdAt.Add(168 * time.Hour)

	query, args, _ := psql.Insert("users").
		Columns("user_id", "username", "nickname", "password", "email", "created_at", "expires_at").
		Values(userId, request.Username, request.Nickname, hashedPassword, request.Email, createdAt, expiresAt).
		ToSql()
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		// Check for constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			// Check which constraint was violated
			if pgErr.ConstraintName == "username_uq" {
				return nil, status.Error(codes.AlreadyExists, "username already exists")
			}
			if pgErr.ConstraintName == "email_uq" {
				return nil, status.Error(codes.AlreadyExists, "email already exists")
			}
		}

		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert user: %v", err)
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pb.User{
		Username: request.Username,
		Nickname: request.Nickname,
		Email:    request.Email,
	}, nil
}
func (as authenticationService) ActivateUser(ctx context.Context, request *pb.ActivateUserRequest) (*pb.User, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Activate not implemented")
}
func (as authenticationService) LoginUser(ctx context.Context, request *pb.LoginUserRequest) (*pb.User, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Login not implemented")
}

func (as authenticationService) UpdatePassword(ctx context.Context, request *pb.ChangePasswordRequest) (*common.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdatePassword not implemented")
}

func generateToken() string {
	rand.NewSource(time.Now().UnixNano())

	// Generate a random 6-digit number
	return strconv.Itoa(rand.Intn(900000) + 100000)
}
