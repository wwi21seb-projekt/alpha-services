package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
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
	// Fetch request from context
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.RegistrationRequest)

	user, err := uh.authService.RegisterUser(c, &pb.RegisterUserRequest{
		Username: req.Username,
		Password: req.Password,
		Nickname: req.Nickname,
		Email:    req.Email,
	})
	if err != nil {
		if err.Error() == "username already exists" {
			c.JSON(409, goerrors.UsernameTaken)
			return
		} else if err.Error() == "email already exists" {
			c.JSON(409, goerrors.EmailTaken)
			return
		}

		c.JSON(500, goerrors.InternalServerError)
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
	// to-do
}

func (uh *UserHandler) LoginUser(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) RefreshToken(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) ActivateUser(c *gin.Context) {
	// to-do
}

func (uh *UserHandler) ResendToken(c *gin.Context) {
	// to-do
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
