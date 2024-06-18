package handler

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/ggwhite/go-masker/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbMail "github.com/wwi21seb-projekt/alpha-shared/proto/mail"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type TokenTypeEnum string

const (
	Activation TokenTypeEnum = "activation"
	Password   TokenTypeEnum = "password_reset"
)

type authenticationService struct {
	logger     *zap.SugaredLogger
	tracer     trace.Tracer
	db         *db.DB
	mailClient pbMail.MailServiceClient
	pb.UnimplementedAuthenticationServiceServer
}

func NewAuthenticationServer(logger *zap.SugaredLogger, database *db.DB, mailClient pbMail.MailServiceClient) pb.AuthenticationServiceServer {
	return &authenticationService{
		logger:     logger,
		tracer:     otel.GetTracerProvider().Tracer("auth-service"),
		db:         database,
		mailClient: mailClient,
	}
}

func (as authenticationService) RegisterUser(ctx context.Context, request *pb.RegisterUserRequest) (*pb.User, error) {
	// Start a transaction
	tx, err := as.db.Begin(ctx)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	_, hashSpan := as.tracer.Start(ctx, "HashPassword")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		as.logger.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		hashSpan.End()
		return nil, status.Errorf(codes.Unknown, "failed to hash password: %v", err)
	}
	hashSpan.End()

	createdAt := time.Now()
	expiresAt := createdAt.Add(168 * time.Hour)

	insertUserCtx, insertUserSpan := as.tracer.Start(ctx, "InsertUser")
	as.logger.Info("Inserting user into database...")
	query, args, _ := psql.Insert("users").
		Columns("username", "nickname", "password", "email", "created_at", "expires_at").
		Values(request.Username, request.Nickname, hashedPassword, request.Email, createdAt, expiresAt).
		ToSql()
	_, err = tx.Exec(insertUserCtx, query, args...)
	if err != nil {
		insertUserSpan.End()
		// Check for constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			as.logger.Info("User creation failed due to unique constraint violation")
			// Check which constraint was violated
			if pgErr.ConstraintName == "users_pkey" {
				return nil, status.Error(codes.AlreadyExists, "username already exists")
			}
			if pgErr.ConstraintName == "email_uq" {
				return nil, status.Error(codes.AlreadyExists, "email already exists")
			}
		}

		as.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to insert user: %v", err)
	}
	insertUserSpan.End()

	// Generate a random 6-digit number
	insertTokenCtx, insertTokenSpan := as.tracer.Start(ctx, "InsertToken")
	activationCode := generateToken()
	tokenId := uuid.New()
	expiresAt = time.Now().Add(24 * time.Hour)
	query, args, _ = psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "username").
		Values(tokenId, activationCode, expiresAt, "activation", request.GetUsername()).
		ToSql()

	as.logger.Info("Inserting activation token into database...")
	_, err = tx.Exec(insertTokenCtx, query, args...)
	if err != nil {
		as.logger.Errorf("Error in tx.Exec: %v", err)
		insertTokenSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}
	insertTokenSpan.End()

	// Call mail service to send registration email
	mailUpstreamCtx, mailUpstreamSpan := as.tracer.Start(ctx, "SendTokenMail")
	as.logger.Info("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(mailUpstreamCtx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_REGISTRATION,
		User: &pbMail.UserInformation{
			Username: request.Username,
			Email:    request.Email,
		},
	})
	if err != nil {
		as.logger.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		mailUpstreamSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}
	mailUpstreamSpan.End()

	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
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
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Fetch the required user data
	queryCtx, querySpan := as.tracer.Start(ctx, "QueryDatabase")
	query, args, _ := psql.Select("activated_at", "email").
		From("users").
		Where("username = ?", request.GetUsername()).
		ToSql()

	var activatedAt *time.Time
	var email string
	as.logger.Info("Querying database for user...")
	err = tx.QueryRow(queryCtx, query, args...).Scan(&activatedAt, &email)
	if err != nil {
		querySpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}
	querySpan.End()

	if activatedAt != nil {
		as.logger.Error("User is already activated")
		return nil, status.Error(codes.FailedPrecondition, "user is already activated")
	}

	// Set a new activation token and send the email
	if err := as.setNewRegistrationTokenAndSendMail(ctx, tx, request.GetUsername(), email); err != nil {
		as.logger.Errorf("Error in as.setNewRegistrationTokenAndSendMail: %v", err)
		return nil, err
	}
	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func (as authenticationService) ActivateUser(ctx context.Context, request *pb.ActivateUserRequest) (*pbCommon.Empty, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Update the user's status to active and return the email
	// for the confirmation email
	updateCtx, updateSpan := as.tracer.Start(ctx, "UpdateUser")
	activatedAt := time.Now()
	query, args, _ := psql.Update("users").
		Set("activated_at", activatedAt).
		Where("username = ?", request.GetUsername()).
		Suffix("RETURNING email, (SELECT activated_at FROM prev)").
		Prefix("WITH prev AS (SELECT activated_at FROM users WHERE username = ?)", request.GetUsername()).
		ToSql()

	var email string
	var alreadyActivated *time.Time
	as.logger.Info("Activating user in database...")
	err = tx.QueryRow(updateCtx, query, args...).Scan(&email, &alreadyActivated)
	if err != nil {
		updateSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Infof("User %s not found", request.GetUsername())
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}
	updateSpan.End()

	if alreadyActivated != nil {
		as.logger.Error("User is already activated")
		return nil, status.Error(codes.FailedPrecondition, "user is already activated")
	}

	// The following query tries deleting the token and returning the user
	// if the user exists and the token is valid, otherwise it returns an error
	deleteCtx, deleteSpan := as.tracer.Start(ctx, "DeleteToken")
	query, args, _ = psql.Delete("tokens").
		Where("token = ? AND type = ? AND username = ?", request.GetToken(), Activation, request.GetUsername()).
		Suffix("RETURNING expires_at").
		ToSql()

	var expiresAt time.Time
	as.logger.Info("Deleting token from database...")
	err = tx.QueryRow(deleteCtx, query, args...).Scan(&expiresAt)
	if err != nil {
		deleteSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("Token not found")
			return nil, status.Error(codes.NotFound, "token not found")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to activate user: %v", err)
	}
	deleteSpan.End()

	if time.Now().After(expiresAt) {
		newTokenCtx, newTokenSpan := as.tracer.Start(ctx, "SendNewToken")
		defer newTokenSpan.End() // we can defer this, since we'll 100% return in this condition
		as.logger.Error("Token expired, sending new token")
		// This is actually quite tricky, since we first need to rollback the ongoing transaction
		// explicit to release the lock on the user row and revert the activation.
		// Then we can start a new transaction and send a new token.
		if err := as.db.Rollback(newTokenCtx, tx); err != nil {
			as.logger.Errorf("Error in db.Rollback: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to rollback transaction: %v", err)
		}

		// We simplify the process by calling the resend token function, even if that produces
		// some unnecessary double-checks but in return handles the new transaction handling.
		if _, err := as.ResendActivationEmail(newTokenCtx, &pb.ResendActivationEmailRequest{
			Username: request.GetUsername(),
		}); err != nil {
			as.logger.Errorf("Error in as.ResendActivationEmail: %v", err)
			return nil, err
		}

		return nil, status.Error(codes.DeadlineExceeded, "token expired")
	}

	// Send confirmation email
	upstreamMailCtx, upstreamMailSpan := as.tracer.Start(ctx, "SendConfirmationMail")
	as.logger.Info("Calling upstream mailClient.SendMail...")
	_, err = as.mailClient.SendConfirmationMail(upstreamMailCtx, &pbMail.ConfirmationMailRequest{
		User: &pbMail.UserInformation{
			Username: request.GetUsername(),
			Email:    email,
		},
	})
	if err != nil {
		as.logger.Errorf("Error in upstream call mailClient.SendMail: %v", err)
		upstreamMailSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to send confirmation mail: %v", err)
	}
	upstreamMailSpan.End()

	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	// No need to return anything, since a nil-error response is a success for the gateway
	return &pbCommon.Empty{}, nil
}

func (as authenticationService) LoginUser(ctx context.Context, request *pb.LoginUserRequest) (*pbCommon.Empty, error) {
	conn, err := as.db.Pool.Acquire(ctx)
	if err != nil {
		as.logger.Errorf("Error in db.Pool.Acquire: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Create a child span for the database query
	dbSpanCtx, dbSpan := as.tracer.Start(ctx, "QueryDatabase")
	query, args, _ := psql.Select("password", "activated_at").
		From("users").
		Where("username = ?", request.Username).
		ToSql()

	var hashedPassword []byte
	var activatedAt pgtype.Timestamptz
	as.logger.Info("Querying database for user...")
	if err := conn.QueryRow(dbSpanCtx, query, args...).Scan(&hashedPassword, &activatedAt); err != nil {
		dbSpan.End()
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}
	dbSpan.End()

	// Check if the user is activated
	if !activatedAt.Valid {
		as.logger.Error("User is not activated")
		return nil, status.Error(codes.FailedPrecondition, "user is not activated")
	}

	// Compare the hashed password with the provided password
	_, passwordSpan := as.tracer.Start(ctx, "ComparePassword")
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(request.Password)); err != nil {
		as.logger.Errorf("Error in bcrypt.CompareHashAndPassword: %v", err)
		passwordSpan.End()
		return nil, status.Error(codes.PermissionDenied, "invalid password")
	}
	passwordSpan.End()

	return &pbCommon.Empty{}, nil
}

func (as authenticationService) UpdatePassword(ctx context.Context, request *pb.ChangePasswordRequest) (*pbCommon.Empty, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Hash the new password
	_, hashSpan := as.tracer.Start(ctx, "HashPassword")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.GetNewPassword()), bcrypt.DefaultCost)
	if err != nil {
		as.logger.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		hashSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}
	hashSpan.End()

	// Get username from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		as.logger.Error("Error in metadata.FromIncomingContext: metadata not found")
		return nil, status.Error(codes.Internal, "metadata not found")
	}
	// Must be present, since the user is logged in
	username, _ := keys.GetMetadataValue(md, keys.SubjectKey)

	// Update the user's password and return the old one to check
	// if the old password is correct and rollback if it is not
	updateCtx, updateSpan := as.tracer.Start(ctx, "UpdateUserPassword")
	query, args, _ := psql.Update("users").
		Set("password", hashedPassword).
		Where("username = ?", username).
		// This CTE does the trick to get the old password, since it captures the state
		// before the update and returns the old password
		Prefix("WITH prev AS (SELECT password FROM users WHERE username = ?)", username).
		Suffix("RETURNING (SELECT password FROM prev)").
		ToSql()

	var oldPassword []byte
	if err := tx.QueryRow(updateCtx, query, args...).Scan(&oldPassword); err != nil {
		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		updateSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to update password: %v", err)
	}
	updateSpan.End()

	// Compare the hashed password with the provided old password
	_, passwordSpan := as.tracer.Start(ctx, "ComparePassword")
	if err := bcrypt.CompareHashAndPassword(oldPassword, []byte(request.GetOldPassword())); err != nil {
		as.logger.Error("Old password is incorrect")
		passwordSpan.End()
		return nil, status.Error(codes.PermissionDenied, "old password is incorrect")
	}
	passwordSpan.End()

	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func (as authenticationService) ResetPassword(ctx context.Context, request *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		as.logger.Errorf("Error in as.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Fetch the required user data
	query, args, _ := psql.Select("email").
		From("users").
		Where("username = ?", request.GetUsername()).
		ToSql()

	var email string
	selectCtx, selectSpan := as.tracer.Start(ctx, "QueryDatabase")
	as.logger.Info("Querying database for user...")
	err = tx.QueryRow(selectCtx, query, args...).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("User not found", zap.Error(err))
			selectSpan.End()
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Error("Error in tx.QueryRow", zap.Error(err))
		selectSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}
	selectSpan.End()

	// Set a new activation token and send the email
	if err := as.setNewPasswordResetTokenAndSendMail(ctx, tx, request.GetUsername(), email); err != nil {
		as.logger.Errorf("Error in as.setNewRegistrationTokenAndSendMail: %v", err)
		return nil, err
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in as.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	// Mask the email
	emailMasker := &masker.EmailMasker{}
	maskedEmail := emailMasker.Marshal("*", email)

	return &pb.ResetPasswordResponse{Email: maskedEmail}, nil
}

func (as authenticationService) SetPassword(ctx context.Context, request *pb.SetPasswordRequest) (*pbCommon.Empty, error) {
	tx, err := as.db.Begin(ctx)
	if err != nil {
		as.logger.Errorf("Error in as.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.Rollback(ctx, tx)

	// Check if the user exists
	query, args, _ := psql.Select("username").
		From("users").
		Where("username = ?", request.GetUsername()).
		ToSql()

	var username string
	as.logger.Info("Checking user in database...")
	err = tx.QueryRow(ctx, query, args...).Scan(&username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to verify user: %v", err)
	}

	// Check if the token is valid and not expired
	query, args, _ = psql.Delete("tokens").
		Where("token = ? AND type = ? AND username = ?", request.GetToken(), Password, request.GetUsername()).
		Suffix("RETURNING expires_at").
		ToSql()

	var expiresAt time.Time
	as.logger.Info("Checking token in database...")
	err = tx.QueryRow(ctx, query, args...).Scan(&expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("Token not found or invalid")
			return nil, status.Error(codes.PermissionDenied, "token not found or invalid")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to verify token: %v", err)
	}

	if time.Now().After(expiresAt) {
		as.logger.Error("Token expired")
		return nil, status.Error(codes.PermissionDenied, "token expired")
	}

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.GetNewPassword()), bcrypt.DefaultCost)
	if err != nil {
		as.logger.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}

	// Update the user's password
	query, args, _ = psql.Update("users").
		Set("password", hashedPassword).
		Where("username = ?", request.GetUsername()).
		ToSql()

	as.logger.Info("Updating password in database...")
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		as.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update password: %v", err)
	}

	if err := as.db.Commit(ctx, tx); err != nil {
		as.logger.Errorf("Error in as.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pbCommon.Empty{}, nil
}

func (as authenticationService) setNewRegistrationTokenAndSendMail(ctx context.Context, tx pgx.Tx, username, email string) error {
	// Start the main span for the setNewRegistrationTokenAndSendMail function
	ctx, span := as.tracer.Start(ctx, "SetNewRegistrationTokenAndSendMail")
	defer span.End()

	// Generate a random 6-digit number
	insertCtx, insertSpan := as.tracer.Start(ctx, "InsertToken")
	activationCode := generateToken()
	tokenId := uuid.New()
	expiresAt := time.Now().Add(24 * time.Hour)
	query, args, _ := psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "username").
		Values(tokenId, activationCode, expiresAt, "activation", username).
		Suffix("ON CONFLICT (username, type) DO UPDATE SET token = ?, expires_at = ?", activationCode, expiresAt).
		ToSql()

	as.logger.Info("Inserting activation token into database...")
	_, err := tx.Exec(insertCtx, query, args...)
	if err != nil {
		as.logger.Errorf("Error in tx.Exec: %v", err)
		insertSpan.End()
		return status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}
	insertSpan.End()

	// Call mail service to send registration email
	upstreamMailCtx, upstreamMailSpan := as.tracer.Start(ctx, "SendTokenMail")
	defer upstreamMailSpan.End() // we can defer here, since we'll return after this
	as.logger.Info("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(upstreamMailCtx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_REGISTRATION,
		User: &pbMail.UserInformation{
			Username: username,
			Email:    email,
		},
	})
	if err != nil {
		as.logger.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		return status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}

	return nil
}

func (as authenticationService) setNewPasswordResetTokenAndSendMail(ctx context.Context, tx pgx.Tx, username, email string) error {
	ctx, span := as.tracer.Start(ctx, "SetNewPasswordResetTokenAndSendMail")
	defer span.End()

	// Generate a random 6-digit number
	insertCtx, insertSpan := as.tracer.Start(ctx, "InsertToken")
	activationCode := generateToken()
	tokenId := uuid.New()
	expiresAt := time.Now().Add(24 * time.Hour)
	query, args, _ := psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "username").
		Values(tokenId, activationCode, expiresAt, Password, username).
		Suffix("ON CONFLICT (username, type) DO UPDATE SET token = ?, expires_at = ?", activationCode, expiresAt).
		ToSql()

	as.logger.Info("Inserting reset password token into database...")
	_, err := tx.Exec(insertCtx, query, args...)
	if err != nil {
		as.logger.Errorf("Error in tx.Exec: %v", err)
		insertSpan.End()
		return status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}
	insertSpan.End()

	// Call mail service to send registration email
	as.logger.Info("Calling upstream mailClient.SendTokenMail...")
	_, err = as.mailClient.SendTokenMail(ctx, &pbMail.TokenMailRequest{
		Token: activationCode,
		Type:  pbMail.TokenMailType_TOKENMAILTYPE_PASSWORD_RESET,
		User: &pbMail.UserInformation{
			Username: username,
			Email:    email,
		},
	})
	if err != nil {
		as.logger.Errorf("Error in upstream call mailClient.SendTokenMail: %v", err)
		return status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}

	return nil
}

func generateToken() string {
	rand.NewSource(time.Now().UnixNano())

	// Generate a random 6-digit number
	return strconv.Itoa(rand.Intn(900000) + 100000)
}
