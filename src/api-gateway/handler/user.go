package handler

import (
	"github.com/gin-gonic/gin"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
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
	profileService      pb.ProfileServiceClient
	subscriptionService pb.SubscriptionServiceClient
}

func NewUserHandler(authService pb.AuthenticationServiceClient, profileService pb.ProfileServiceClient, subscriptionService pb.SubscriptionServiceClient) *UserHandler {
	return &UserHandler{
		authService:         authService,
		profileService:      profileService,
		subscriptionService: subscriptionService,
	}
}

func (h *UserHandler) RegisterUser(c *gin.Context) {
	// to-do
}

func (h *UserHandler) SearchUsers(c *gin.Context) {
	// to-do
}

func (h *UserHandler) ChangeTrivialInfo(c *gin.Context) {
	// to-do
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	// to-do
}

func (h *UserHandler) LoginUser(c *gin.Context) {
	// to-do
}

func (h *UserHandler) RefreshToken(c *gin.Context) {
	// to-do
}

func (h *UserHandler) ActivateUser(c *gin.Context) {
	// to-do
}

func (h *UserHandler) ResendToken(c *gin.Context) {
	// to-do
}

func (h *UserHandler) GetUserFeed(c *gin.Context) {
	// to-do
}

func (h *UserHandler) GetUser(c *gin.Context) {
	// to-do
}

func (h *UserHandler) CreateSubscription(c *gin.Context) {
	// to-do
}

func (h *UserHandler) DeleteSubscription(c *gin.Context) {
	// to-do
}

func (h *UserHandler) GetSubscriptions(c *gin.Context) {
	// to-do
}
