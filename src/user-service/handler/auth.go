package handler

import (
	"context"
	"errors"
	"google.golang.org/grpc/metadata"
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
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type TokenTypeEnum string

const (
	Activation TokenTypeEnum = "activation"
	Password   TokenTypeEnum = "password"
)

type authenticationService struct {
	db         *db.DB
	mailClient pbMail.MailServiceClient
	pb.UnimplementedAuthenticationServiceServer
}

func NewAuthenticationServer(database *db.DB, mailClient pbMail.MailServiceClient) pb.AuthenticationServiceServer {
	return &authenticationService{
		db:         database,
		mailClient: mailClient,
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

	log.Println("Inserting user into database...")
	query, args, _ := psql.Insert("users").
		Columns("user_id", "username", "nickname", "password", "email", "created_at", "expires_at").
		Values(userId, request.Username, request.Nickname, hashedPassword, request.Email, createdAt, expiresAt).
		ToSql()
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		// Check for constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			log.Println("User creation failed due to unique constraint violation")
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

	// Generate a random 6-digit number
	activationCode := generateToken()
	tokenId := uuid.New()
	expiresAt = time.Now().Add(24 * time.Hour)
	query, args, _ = psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "user_id").
		Values(tokenId, activationCode, expiresAt, "activation", userId).
		ToSql()

	// Call mail service to send registration email
	log.Println("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(ctx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_REGISTRATION,
		User: &pb.User{
			Username: request.Username,
			Email:    request.Email,
		},
	})
	if err != nil {
		log.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}

	log.Println("Inserting activation token into database...")
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert token: %v", err)

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
	tx, err := as.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Get username from metadata (will be placed in the request in the future)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Error("Error in metadata.FromIncomingContext: missing metadata")
		return nil, status.Error(codes.FailedPrecondition, "missing metadata")
	}
	username := md.Get("username")[0]

	// The following query tries deleting the token and returning the user
	// if the user exists and the token is valid, otherwise it returns an error
	query, args, _ := psql.Delete("tokens").
		Where("token = ?", request.Token).
		Where("expires_at > NOW()").
		Where("user_id IN (SELECT user_id FROM users WHERE username = ?)", username).
		Suffix("RETURNING user_id").
		ToSql()

	var userId uuid.UUID
	err = tx.QueryRow(ctx, query, args...).Scan(&userId)
	if err != nil {
		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}

	// Update the user's status to active and return the email
	// for the confirmation email
	query, args, _ = psql.Update("users").
		Set("active", true).
		Where("user_id = ?", userId).
		Suffix("RETURNING username, email").
		ToSql()

	var email string
	err = tx.QueryRow(ctx, query, args...).Scan(&email)
	if err != nil {
		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}

	// Send confirmation email
	log.Println("Calling upstream mailClient.SendMail...")
	_, err = as.mailClient.SendConfirmationMail(ctx, &pbMail.ConfirmationMailRequest{
		User: &pb.User{
			Username: username,
			Email:    email,
		},
	})
	if err != nil {
		log.Errorf("Error in upstream call mailClient.SendMail: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to send confirmation mail: %v", err)
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	// We actually don't need to return anything here, will be changed in the future
	return nil, nil
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
