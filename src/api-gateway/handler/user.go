package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type UserHdlr interface {
	RegisterUser(c *gin.Context)       // POST /users
	SearchUsers(c *gin.Context)        // GET /users
	ChangeTrivialInfo(c *gin.Context)  // PUT /users
	ChangePassword(c *gin.Context)     // PATCH /users
	LoginUser(c *gin.Context)          // POST /users/login
	RefreshToken(c *gin.Context)       // POST /users/refresh
	ActivateUser(c *gin.Context)       // POST /users/:username/activate
	ResendToken(c *gin.Context)        // DELETE /users/:username/activate
	GetUserFeed(c *gin.Context)        // GET /users/:username/feed
	GetUser(c *gin.Context)            // GET /users/:username
	CreateSubscription(c *gin.Context) // POST /subscriptions
	DeleteSubscription(c *gin.Context) // DELETE /subscriptions/:subscriptionId
	GetSubscriptions(c *gin.Context)   // GET /subscriptions/:username
}

type UserHandler struct {
	authService         pb.AuthenticationServiceClient
	profileService      pb.UserServiceClient
	subscriptionService pb.SubscriptionServiceClient
}

func NewUserHandler(authService pb.AuthenticationServiceClient, profileService pb.UserServiceClient, subscriptionService pb.SubscriptionServiceClient) *UserHandler {
	return &UserHandler{
		authService:         authService,
		profileService:      profileService,
		subscriptionService: subscriptionService,
	}
}

func (uh *UserHandler) RegisterUser(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.RegistrationRequest)

	user, err := uh.authService.RegisterUser(c, &pb.RegisterUserRequest{
		Username: req.Username,
		Password: req.Password,
		Nickname: req.Nickname,
		Email:    req.Email,
	})
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		if rpcStatus.Code() == codes.AlreadyExists {
			if rpcStatus.Message() == "username already exists" {
				returnErr = goerrors.UsernameTaken
			} else if rpcStatus.Message() == "email already exists" {
				returnErr = goerrors.EmailTaken
			}
		} else if rpcStatus.Code() == codes.InvalidArgument {
			// AuthService currently does not return this error, but will be added in the future
			// so this is a placeholder for now
			returnErr = goerrors.EmailUnreachable
		}

		log.Printf("Error in upstream call uh.authService.RegisterUser: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	c.JSON(201, user)
}

func (uh *UserHandler) SearchUsers(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) ChangeTrivialInfo(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) ChangePassword(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.ChangePasswordRequest)

	// Write user id in metadata
	userId, _ := keys.GetClaim(c, keys.SubjectKey)
	ctx := metadata.NewOutgoingContext(c.Request.Context(), metadata.Pairs(string(keys.SubjectKey), userId))

	_, err := uh.authService.UpdatePassword(ctx, &pb.ChangePasswordRequest{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.PermissionDenied {
			returnErr = goerrors.InvalidCredentials
		}

		log.Printf("Error in upstream call uh.authService.UpdatePassword: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	c.JSON(204, nil)
}

func (uh *UserHandler) LoginUser(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.LoginRequest)

	userId, err := uh.authService.LoginUser(c, &pb.LoginUserRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.FailedPrecondition {
			returnErr = goerrors.UserNotActivated
		} else if code == codes.PermissionDenied {
			returnErr = goerrors.InvalidCredentials
		} else if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		}

		log.Printf("Error in upstream call uh.authService.LoginUser: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	// TO-DO: Generate JWT and send it back
	c.JSON(200, userId)
}

func (uh *UserHandler) RefreshToken(c *gin.Context) {
	// to-do
	c.JSON(http.StatusNotImplemented, nil)
}

func (uh *UserHandler) ActivateUser(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.ActivationRequest)
	username := c.Param("username")

	log.Printf("Calling upstream service uh.authService.ActivateUser with username %s and token %s", username, req.Token)
	userId, err := uh.authService.ActivateUser(c, &pb.ActivateUserRequest{
		Username: username,
		Token:    req.Token,
	})
	if err != nil {
		rpcStatus := status.Convert(err)
		returnErr := goerrors.InternalServerError

		if rpcStatus.Code() == codes.NotFound {
			if rpcStatus.Message() == "user not found" {
				returnErr = goerrors.UserNotFound
			} else if rpcStatus.Message() == "token not found" {
				returnErr = goerrors.InvalidToken
			}
		} else if rpcStatus.Code() == codes.DeadlineExceeded {
			returnErr = goerrors.ActivationTokenExpired
		} else if rpcStatus.Code() == codes.FailedPrecondition {
			returnErr = goerrors.UserAlreadyActivated
		}

		log.Printf("Error in upstream call uh.authService.ActivateUser: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	// TO-DO: Generate JWT and send it back
	log.Printf("User %s activated", userId)
	c.JSON(200, userId)
}

func (uh *UserHandler) ResendToken(c *gin.Context) {
	username := c.Param("username")

	log.Printf("Calling upstream service uh.authService.ResendToken with username %s", username)
	_, err := uh.authService.ResendActivationEmail(c, &pb.ResendActivationEmailRequest{
		Username: username,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		} else if code == codes.FailedPrecondition {
			returnErr = goerrors.UserAlreadyActivated
		}

		log.Printf("Error in upstream call uh.authService.ResendToken: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	c.JSON(204, nil)
}

func (uh *UserHandler) GetUserFeed(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) GetUser(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) CreateSubscription(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) DeleteSubscription(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) GetSubscriptions(c *gin.Context) {
	// to-do
}
