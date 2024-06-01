package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"google.golang.org/grpc/codes"
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
	jwtManager          manager.JWTManager
}

func NewUserHandler(authService pb.AuthenticationServiceClient, profileService pb.UserServiceClient, subscriptionService pb.SubscriptionServiceClient, jwtManager manager.JWTManager) *UserHandler {
	return &UserHandler{
		authService:         authService,
		profileService:      profileService,
		subscriptionService: subscriptionService,
		jwtManager:          jwtManager,
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
	username := c.Query("username")
	offset, limit := helper.ExtractPaginationFromContext(c)

	users, err := uh.profileService.SearchUsers(c, &pb.SearchUsersRequest{
		Query: username,
		Pagination: &pbCommon.PaginationRequest{
			Offset: int32(offset),
			Limit:  int32(limit),
		},
	})
	if err != nil {
		log.Printf("Error in upstream call uh.profileService.SearchUsers: %v", err)
		c.JSON(goerrors.InternalServerError.HttpStatus, goerrors.InternalServerError)
		return
	}

	response := &schema.SearchUsersResponse{
		Users: make([]schema.Author, len(users.Users)),
		Pagination: &schema.PaginationResponse{
			Records: users.Pagination.Records,
			Offset:  users.Pagination.Offset,
			Limit:   users.Pagination.Limit,
		},
	}

	// Convert users to schema.Author
	for i, user := range users.Users {
		response.Users[i] = *helper.TransformUser(user)
	}

	c.JSON(200, response)
}

func (uh *UserHandler) ChangeTrivialInfo(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.ChangeTrivialInformationRequest)

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := uh.profileService.UpdateUser(ctx, &pb.UpdateUserRequest{
		Nickname: req.NewNickname,
		Status:   req.Status,
	})
	if err != nil {
		log.Printf("Error in upstream call uh.profileService.UpdateUser: %v", err)
		c.JSON(goerrors.InternalServerError.HttpStatus, goerrors.InternalServerError)
		return
	}

	c.JSON(200, &schema.ChangeTrivialInformamtionResponse{
		Nickname: req.NewNickname,
		Status:   req.Status,
	})
}

func (uh *UserHandler) ChangePassword(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.ChangePasswordRequest)

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

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

	_, err := uh.authService.LoginUser(c, &pb.LoginUserRequest{
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

	tokenPair, err := uh.jwtManager.Generate(req.Username)
	if err != nil {
		log.Printf("Error in jwtManager.Generate: %v", err)
		c.JSON(goerrors.InternalServerError.HttpStatus, goerrors.InternalServerError)
		return
	}

	c.JSON(http.StatusOK, tokenPair)
}

func (uh *UserHandler) RefreshToken(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.RefreshTokenRequest)

	_, err := uh.jwtManager.Verify(req.RefreshToken)
	if err != nil {
		log.Printf("Error in jwtManager.Verify: %v", err)
		c.JSON(goerrors.InvalidToken.HttpStatus, goerrors.InvalidToken)
		return
	}

	tokenPair, err := uh.jwtManager.Refresh(req.RefreshToken)
	if err != nil {
		log.Printf("Error in jwtManager.Refresh: %v", err)
		c.JSON(goerrors.InternalServerError.HttpStatus, goerrors.InternalServerError)
		return
	}
	c.JSON(http.StatusOK, tokenPair)
}

func (uh *UserHandler) ActivateUser(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.ActivationRequest)
	username := c.Param("username")

	log.Printf("Calling upstream service uh.authService.ActivateUser with username %s and token %s", username, req.Token)
	_, err := uh.authService.ActivateUser(c, &pb.ActivateUserRequest{
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

	tokenPair, err := uh.jwtManager.Generate(username)
	if err != nil {
		log.Printf("Error in jwtManager.Generate: %v", err)
		c.JSON(goerrors.InternalServerError.HttpStatus, goerrors.InternalServerError)
		return
	}

	log.Printf("User %s activated", username)
	c.JSON(200, tokenPair)
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
	username := c.Param("username")

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	user, err := uh.profileService.GetUser(ctx, &pb.GetUserRequest{
		Username: username,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		}

		log.Printf("Error in upstream call uh.profileService.GetUser: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	response := &schema.GetUserResponse{
		Username:          username,
		Nickname:          user.Nickname,
		Status:            user.Status,
		ProfilePictureUrl: user.ProfilePictureUrl,
		FollowerCount:     user.FollowerCount,
		FollowingCount:    user.FollowingCount,
		PostCount:         user.PostCount,
		SubscriptionId:    user.SubscriptionId,
	}

	c.JSON(200, response)
}

func (uh *UserHandler) CreateSubscription(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.SubscriptionRequest)

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	res, err := uh.subscriptionService.CreateSubscription(ctx, &pb.CreateSubscriptionRequest{
		FollowedUsername: req.Following,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		} else if code == codes.AlreadyExists {
			returnErr = goerrors.SubscriptionAlreadyExists
		} else if code == codes.InvalidArgument {
			returnErr = goerrors.SubscriptionSelfFollow
		}

		log.Printf("Error in upstream call uh.subscriptionService.CreateSubscription: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	response := &schema.CreateSubscriptionResponse{
		SubscriptionId:    res.SubscriptionId,
		SubscriptionDate:  res.SubscriptionDate,
		FollowerUsername:  res.FollowerUsername,
		FollowingUsername: res.FollowedUsername,
	}

	c.JSON(201, response)
}

func (uh *UserHandler) DeleteSubscription(c *gin.Context) {
	subscriptionId := c.Param("subscriptionId")

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := uh.subscriptionService.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{
		SubscriptionId: subscriptionId,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.SubscriptionNotFound
		} else if code == codes.PermissionDenied {
			returnErr = goerrors.UnsubscribeForbidden
		}

		log.Printf("Error in upstream call uh.subscriptionService.DeleteSubscription: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	c.JSON(204, nil)
}

func (uh *UserHandler) GetSubscriptions(c *gin.Context) {
	username := c.Param("username")
	offset, limit := helper.ExtractPaginationFromContext(c)

	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	subType := pb.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWER
	if c.Query("type") == "following" {
		subType = pb.SubscriptionType_SUBSCRIPTION_TYPE_FOLLOWING
	}

	subscriptions, err := uh.subscriptionService.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{
		Username:         username,
		SubscriptionType: subType,
		Pagination: &pbCommon.PaginationRequest{
			Offset: int32(offset),
			Limit:  int32(limit),
		},
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		}

		log.Printf("Error in upstream call uh.subscriptionService.GetSubscriptions: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	response := &schema.GetSubscriptionsResponse{
		Subscriptions: make([]schema.UserSubscription, len(subscriptions.Subscriptions)),
		Pagination: &schema.PaginationResponse{
			Records: subscriptions.Pagination.Records,
			Offset:  subscriptions.Pagination.Offset,
			Limit:   subscriptions.Pagination.Limit,
		},
	}

	// Convert subscriptions to schema.Subscription
	for i, sub := range subscriptions.Subscriptions {
		response.Subscriptions[i] = *helper.TransformUserSubscription(sub)
	}

	c.JSON(200, response)
}
