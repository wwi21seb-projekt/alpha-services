package handler

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"

	sq "github.com/Masterminds/squirrel"
	"github.com/ggwhite/go-masker/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

const (
	defaultAvatarURL    = "https://getdrawings.com/free-icon/default-avatar-icon-62.png"
	defaultAvatarWidth  = int32(512)
	defaultAvatarHeight = int32(512)
)

type TokenTypeEnum string

const (
	Activation TokenTypeEnum = "activation"
	Password   TokenTypeEnum = "password_reset"
)

type authenticationService struct {
	logger      *zap.SugaredLogger
	tracer      trace.Tracer
	db          *db.DB
	mailClient  notificationv1.MailServiceClient
	imageClient imagev1.ImageServiceClient
	userv1.UnimplementedAuthenticationServiceServer
}

func NewAuthenticationServer(logger *zap.SugaredLogger, database *db.DB, mailClient notificationv1.MailServiceClient, imageClient imagev1.ImageServiceClient) userv1.AuthenticationServiceServer {
	return &authenticationService{
		logger:      logger,
		tracer:      otel.GetTracerProvider().Tracer("auth-service"),
		db:          database,
		mailClient:  mailClient,
		imageClient: imageClient,
	}
}

func (as authenticationService) RegisterUser(ctx context.Context, request *userv1.RegisterUserRequest) (*userv1.RegisterUserResponse, error) {
	// Hash the password
	_, hashSpan := as.tracer.Start(ctx, "HashPassword")
	as.logger.Debug("Hashing user password")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		as.logger.Errorf("Error in bcrypt.GenerateFromPassword: %v", err)
		hashSpan.End()
		return nil, status.Errorf(codes.Unknown, "failed to hash password: %v", err)
	}
	hashSpan.End()

	// Start a transaction
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

	// Check for existing user or email
	query, args, err := psql.Select("username, email").
		From("users").
		Where(sq.Or{sq.Eq{"username": request.GetUsername()}, sq.Eq{"email": request.GetEmail()}}).
		ToSql()
	if err != nil {
		as.logger.Warnw("Error in psql.Select", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}

	var username string
	var email string
	as.logger.Debug("Checking if user or email exists")
	err = tx.QueryRow(ctx, query, args...).Scan(&username, &email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			as.logger.Warnw("Error in tx.QueryRow", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
		}
		// User does not exist, continue
	} else {
		if username == request.GetUsername() {
			as.logger.Debug("Username already exists")
			return nil, status.Error(codes.AlreadyExists, "username already exists")
		}
		if email == request.GetEmail() {
			as.logger.Debug("Email already exists")
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}
	}

	// Upload image to storage if provided
	imageUrl := defaultAvatarURL
	imageWidth := defaultAvatarWidth
	imageHeight := defaultAvatarHeight

	if request.GetImage() != "" {
		uploadImageCtx, uploadImageSpan := as.tracer.Start(ctx, "UploadImage")
		as.logger.Infow("Uploading image to storage", "username", request.GetUsername())
		uploadImageResponse, err := as.imageClient.UploadImage(uploadImageCtx, &imagev1.UploadImageRequest{
			Image: request.GetImage(),
			Name:  request.GetUsername(),
		})
		if err != nil {
			as.logger.Errorf("Error in upstream call imageClient.UploadImage: %v", err)
			uploadImageSpan.End()
			return nil, err
		}

		imageUrl = uploadImageResponse.GetUrl()
		imageWidth = uploadImageResponse.GetWidth()
		imageHeight = uploadImageResponse.GetHeight()

		uploadImageSpan.End()
	}

	createdAt := time.Now()
	expiresAt := createdAt.Add(168 * time.Hour)

	as.logger.Debugw("Inserting user into database", "username", request.Username)
	query, args, _ = psql.Insert("users").
		Columns("username", "nickname", "password", "email", "picture_url", "picture_width", "picture_height", "created_at", "expires_at").
		Values(request.Username, request.Nickname, hashedPassword, request.Email, imageUrl, imageWidth, imageHeight, createdAt, expiresAt).
		ToSql()
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		as.logger.Errorw("Error while inserting user into database", "error", err, "username", request.Username)
		return nil, status.Errorf(codes.Internal, "failed to insert user: %v", err)
	}

	// Generate a random 6-digit number
	activationCode := generateToken()
	tokenId := uuid.New()
	expiresAt = time.Now().Add(24 * time.Hour)
	query, args, _ = psql.Insert("tokens").
		Columns("token_id", "token", "expires_at", "type", "username").
		Values(tokenId, activationCode, expiresAt, "activation", request.GetUsername()).
		ToSql()

	as.logger.Debugw("Inserting activation token into database", "username", request.Username)
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		as.logger.Errorw("Error in tx.Exec while inserting token", "error", err, "username", request.Username)
		return nil, status.Errorf(codes.Internal, "failed to insert token: %v", err)
	}

	// Call mail service to send registration email
	mailUpstreamCtx, mailUpstreamSpan := as.tracer.Start(ctx, "SendTokenMail")
	as.logger.Infow("Calling upstream mailClient.SendTokenMail", "username", request.Username)
	_, err = as.mailClient.SendTokenMail(mailUpstreamCtx, &notificationv1.SendTokenMailRequest{
		Token: activationCode,
		Type:  notificationv1.TokenMailType_TOKEN_MAIL_TYPE_REGISTRATION,
		User: &notificationv1.UserInformation{
			Username: request.Username,
			Email:    request.Email,
		},
	})
	if err != nil {
		as.logger.Errorw("Error in upstream call mailClient.SendTokenMail while sending token mail", "error", err, "username", request.Username, "email", request.Email)
		mailUpstreamSpan.End()
		return nil, status.Errorf(codes.Internal, "failed to send token mail: %v", err)
	}
	mailUpstreamSpan.End()

	if err = as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorw("Error in tx.Commit while committing transaction", "error", err, "username", request.Username)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	as.logger.Infow("User registered successfully", "username", request.Username)
	return &userv1.RegisterUserResponse{
		Username: request.Username,
		Nickname: request.Nickname,
		Email:    request.Email,
		Picture: &imagev1.Picture{
			Url:    imageUrl,
			Width:  imageWidth,
			Height: imageHeight,
		},
	}, nil
}

func (as authenticationService) ResendActivationEmail(ctx context.Context, request *userv1.ResendActivationEmailRequest) (*userv1.ResendActivationEmailResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

	// Fetch the required user data
	query, args, _ := psql.Select("activated_at", "email").
		From("users").
		Where("username = ?", request.GetUsername()).
		ToSql()

	var activatedAt *time.Time
	var email string
	as.logger.Info("Querying database for user...")
	err = tx.QueryRow(ctx, query, args...).Scan(&activatedAt, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Debugw("User not found", "username", request.GetUsername())
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in tx.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}

	if activatedAt != nil {
		as.logger.Debugw("User is already activated", "username", request.GetUsername())
		return nil, status.Error(codes.FailedPrecondition, "user is already activated")
	}

	// Set a new activation token and send the email
	if err := as.setNewRegistrationTokenAndSendMail(ctx, tx, request.GetUsername(), email); err != nil {
		as.logger.Errorw("Error in as.setNewRegistrationTokenAndSendMail", "error", err)
		return nil, err
	}
	if err := as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorw("Error in tx.Commit", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &userv1.ResendActivationEmailResponse{}, nil
}

func (as authenticationService) ActivateUser(ctx context.Context, request *userv1.ActivateUserRequest) (*userv1.ActivateUserResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

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
		Where("token = ? AND type = ? AND username = ?", request.GetActivationCode(), Activation, request.GetUsername()).
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
		if err := as.db.RollbackTx(newTokenCtx, tx); err != nil {
			as.logger.Errorf("Error in db.Rollback: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to rollback transaction: %v", err)
		}

		// We simplify the process by calling the resend token function, even if that produces
		// some unnecessary double-checks but in return handles the new transaction handling.
		if _, err := as.ResendActivationEmail(newTokenCtx, &userv1.ResendActivationEmailRequest{
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
	_, err = as.mailClient.SendConfirmationMail(upstreamMailCtx, &notificationv1.SendConfirmationMailRequest{
		User: &notificationv1.UserInformation{
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

	if err := as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	// No need to return anything, since a nil-error response is a success for the gateway
	return &userv1.ActivateUserResponse{}, nil
}

func (as authenticationService) LoginUser(ctx context.Context, request *userv1.LoginUserRequest) (*userv1.LoginUserResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		as.logger.Errorf("Error in db.Pool.Acquire: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to acquire connection: %v", err)
	}
	defer conn.Release()

	// Create a child span for the database query
	query, args, _ := psql.Select("password", "activated_at").
		From("users").
		Where("username = ?", request.Username).
		ToSql()

	var hashedPassword []byte
	var activatedAt pgtype.Timestamptz
	as.logger.Info("Querying database for user...")
	if err := conn.QueryRow(ctx, query, args...).Scan(&hashedPassword, &activatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			as.logger.Error("User not found")
			return nil, status.Error(codes.NotFound, "user not found")
		}

		as.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to query database: %v", err)
	}

	// Check if the user is activated
	if !activatedAt.Valid {
		as.logger.Debugw("User is not activated", "username", request.Username)
		return nil, status.Error(codes.FailedPrecondition, "user is not activated")
	}

	// Compare the hashed password with the provided password
	_, passwordSpan := as.tracer.Start(ctx, "ComparePassword")
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(request.Password)); err != nil {
		as.logger.Debugw("Invalid password", "username", request.Username)
		passwordSpan.End()
		return nil, status.Error(codes.PermissionDenied, "invalid password")
	}
	passwordSpan.End()

	return &userv1.LoginUserResponse{}, nil
}

func (as authenticationService) UpdatePassword(ctx context.Context, request *userv1.UpdatePasswordRequest) (*userv1.UpdatePasswordResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

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

	if err := as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorf("Error in tx.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &userv1.UpdatePasswordResponse{}, nil
}

func (as authenticationService) ResetPassword(ctx context.Context, request *userv1.ResetPasswordRequest) (*userv1.ResetPasswordResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in as.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

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

	if err := as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorf("Error in as.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	// Mask the email
	emailMasker := &masker.EmailMasker{}
	maskedEmail := emailMasker.Marshal("*", email)

	return &userv1.ResetPasswordResponse{Email: maskedEmail}, nil
}

func (as authenticationService) SetPassword(ctx context.Context, request *userv1.SetPasswordRequest) (*userv1.SetPasswordResponse, error) {
	conn, err := as.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := as.db.BeginTx(ctx, conn)
	if err != nil {
		as.logger.Errorf("Error in as.db.Begin: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", err)
	}
	defer as.db.RollbackTx(ctx, tx)

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

	if err := as.db.CommitTx(ctx, tx); err != nil {
		as.logger.Errorf("Error in as.db.Commit: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &userv1.SetPasswordResponse{}, nil
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
	_, err = as.mailClient.SendTokenMail(upstreamMailCtx, &notificationv1.SendTokenMailRequest{
		Token: activationCode,
		Type:  notificationv1.TokenMailType_TOKEN_MAIL_TYPE_REGISTRATION,
		User: &notificationv1.UserInformation{
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
	_, err = as.mailClient.SendTokenMail(ctx, &notificationv1.SendTokenMailRequest{
		Token: activationCode,
		Type:  notificationv1.TokenMailType_TOKEN_MAIL_TYPE_PASSWORD_RESET,
		User: &notificationv1.UserInformation{
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
