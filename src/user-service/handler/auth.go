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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

	log.Println("Inserting activation token into database...")
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}

	// Call mail service to send registration email
	log.Println("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(ctx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_REGISTRATION,
		User: &pbMail.UserInformation{
			Username: request.Username,
			Email:    request.Email,
		},
	})
	if err != nil {
		log.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to send token mail: %v", err)
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

func (as authenticationService) ResendActivationEmail(ctx context.Context, request *pb.ResendActivationEmailRequest) (*pbCommon.Empty, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Fetch the required user data
	query, args, _ := psql.Select("user_id", "activated_at", "email").
		From("users").
		Where("username = ?", request.GetUsername()).
		ToSql()

	var userId uuid.UUID
	var activatedAt *time.Time
	var email string
	log.Println("Querying database for user...")
	err = tx.QueryRow(ctx, query, args...).Scan(&userId, &activatedAt, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}

	if activatedAt != nil {
		log.Error("User is already activated")
		return nil, status.Error(codes.FailedPrecondition, "user is already activated")
	}

	// Generate a random 6-digit number
	tokenId := uuid.New()
	activationCode := generateToken()
	expires_at := time.Now().Add(24 * time.Hour)

	// Update the token in the database
	query, args, _ = psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "user_id").
		Values(tokenId, activationCode, expires_at, Activation, userId).
		Suffix("ON CONFLICT (user_id, type) DO UPDATE SET token = ?, expires_at = ?", activationCode, expires_at).
		ToSql()

	log.Println("Inserting activation token into database...")
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}

	// Call mail service to send registration email
	log.Println("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(ctx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_REGISTRATION,
		User: &pbMail.UserInformation{
			Username: request.GetUsername(),
			Email:    email,
		},
	})

	if err != nil {
		log.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func (as authenticationService) ActivateUser(ctx context.Context, request *pb.ActivateUserRequest) (*pb.UserIdResponse, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Update the user's status to active and return the email
	// for the confirmation email
	activatedAt := time.Now()
	query, args, _ := psql.Update("users").
		Set("activated_at", activatedAt).
		Where("username = ?", request.GetUsername()).
		Suffix("RETURNING user_id, email", "already_activated").
		Prefix("WITH already_activated AS (SELECT activated_at FROM users WHERE username = ?)", request.GetUsername()).
		ToSql()

	var email string
	var userId uuid.UUID
	var alreadyActivated *time.Time
	log.Println("Activating user in database...")
	err = tx.QueryRow(ctx, query, args...).Scan(&userId, &email, &alreadyActivated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}

	if alreadyActivated != nil {
		log.Error("User is already activated")
		return nil, status.Error(codes.FailedPrecondition, "user is already activated")
	}

	// The following query tries deleting the token and returning the user
	// if the user exists and the token is valid, otherwise it returns an error
	query, args, _ = psql.Delete("tokens").
		Where("token = ?", request.GetToken()).
		Where("type = ?", Activation).
		Where("user_id = ?)", userId).
		Suffix("RETURNING expires_at").
		ToSql()

	var expiresAt time.Time
	log.Println("Deleting token from database...")
	err = tx.QueryRow(ctx, query, args...).Scan(&expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Error("Token not found")
			return nil, status.Error(codes.NotFound, "token not found")
		}

		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}

	if time.Now().After(expiresAt) {
		log.Error("Token expired")
		return nil, status.Error(codes.DeadlineExceeded, "token expired")
	}

	// Send confirmation email
	log.Println("Calling upstream mailClient.SendMail...")
	_, err = as.mailClient.SendConfirmationMail(ctx, &pbMail.ConfirmationMailRequest{
		User: &pbMail.UserInformation{
			Username: request.GetUsername(),
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

	// We only need to return the userID, so the gateway can create a jwt token
	// for the user with the userID as the subject
	return &pb.UserIdResponse{
		UserId: userId.String(),
	}, nil
}

func (as authenticationService) LoginUser(ctx context.Context, request *pb.LoginUserRequest) (*pb.UserIdResponse, error) {
	conn, err := as.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("Error in db.Pool.Acquire: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to acquire connection: %v", err)
	}
	defer conn.Release()

	query, args, _ := psql.Select("user_id", "password", "activated_at").
		From("users").
		Where("username = ?", request.Username).
		ToSql()

	var userId uuid.UUID
	var hashedPassword []byte
	var activatedAt time.Time
	log.Println("Querying database for user...")
	if err := conn.QueryRow(ctx, query, args...).Scan(&userId, &hashedPassword, &activatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		log.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}

	// Check if the user is activated
	if activatedAt.IsZero() {
		log.Error("User is not activated")
		return nil, status.Error(codes.FailedPrecondition, "user is not activated")
	}

	// Compare the hashed password with the provided password
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(request.Password)); err != nil {
		log.Errorf("Error in bcrypt.CompareHashAndPassword: %v", err)
		return nil, status.Error(codes.PermissionDenied, "invalid password")
	}

	return &pb.UserIdResponse{
		UserId: userId.String(),
	}, nil
}

func (as authenticationService) UpdatePassword(ctx context.Context, request *pb.ChangePasswordRequest) (*pbCommon.Empty, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		log.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.GetNewPassword()), bcrypt.DefaultCost)
	if err != nil {
		log.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}

	// Get userId from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Error("Error in metadata.FromIncomingContext: metadata not found")
		return nil, status.Error(codes.Internal, "metadata not found")
	}
	// Must be present, since the user is logged in
	userId, _ := keys.GetMetadataValue(md, keys.SubjectKey)

	// Update the user's password and return the old one to check
	// if the old password is correct and rollback if it is not
	query, args, _ := psql.Update("users").
		Set("password", hashedPassword).
		Where("user_id = ?", userId).
		// This CTE does the trick to get the old password, since it captures the state
		// before the update and returns the old password
		Prefix("WITH old_password AS (SELECT password FROM users WHERE user_id = ?)", userId).
		Suffix("RETURNING old_password").
		ToSql()

	var oldPassword []byte
	if err := tx.QueryRow(ctx, query, args...).Scan(&oldPassword); err != nil {
		log.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update password: %v", err)
	}

	// Compare the hashed password with the provided old password
	if err := bcrypt.CompareHashAndPassword(oldPassword, []byte(request.GetOldPassword())); err != nil {
		log.Error("Old password is incorrect")
		return nil, status.Error(codes.PermissionDenied, "old password is incorrect")
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func generateToken() string {
	rand.NewSource(time.Now().UnixNano())

	// Generate a random 6-digit number
	return strconv.Itoa(rand.Intn(900000) + 100000)
}
