package handler

import (
	"context"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"

	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type NotificationHdlr interface {
	GetPublicKey(c *gin.Context)           // GET /push/vapid
	CreatePushSubscription(c *gin.Context) // POST /push/register
	GetNotifications(c *gin.Context)       // GET /notifications
	DeleteNotification(c *gin.Context)     // DELETE /notifications/:notificationId

}

type NotificationHandler struct {
	logger                  *zap.SugaredLogger
	notificationService     notificationv1.NotificationServiceClient
	pushSubscriptionService notificationv1.PushServiceClient
}

func NewNotificationHandler(logger *zap.SugaredLogger, notificationClient notificationv1.NotificationServiceClient, pushSubscriptionClient notificationv1.PushServiceClient) NotificationHdlr {
	return &NotificationHandler{
		logger:                  logger,
		notificationService:     notificationClient,
		pushSubscriptionService: pushSubscriptionClient,
	}
}

func (n *NotificationHandler) GetPublicKey(c *gin.Context) {
	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	publicKey, err := n.pushSubscriptionService.GetPublicKey(ctx, &notificationv1.GetPublicKeyRequest{})

	if err != nil {
		returnErr := goerrors.InternalServerError

		n.logger.Infof("Error in upstream call n.pushSubscriptionService.GetPublicKey: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}
	response := schema.GetPublicKeyResponse{
		Key: publicKey.PublicKey,
	}
	c.JSON(200, response)
}

func (n *NotificationHandler) CreatePushSubscription(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.CreatePushSubscriptionRequest)

	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	pushType := notificationv1.PushSubscriptionType_PUSH_SUBSCRIPTION_TYPE_WEB
	if req.Type == "expo" {
		pushType = notificationv1.PushSubscriptionType_PUSH_SUBSCRIPTION_TYPE_EXPO
	}

	// Initialisiere gRPC-Request
	grpcReq := &notificationv1.CreatePushSubscriptionRequest{
		Type:  pushType,
		Token: req.Token,
	}

	// Fülle die Web-spezifischen Felder, falls der Typ "web" ist
	if req.Type == "web" && req.Subscription != nil {
		grpcReq.Endpoint = req.Subscription.Endpoint
		grpcReq.ExpirationTime = ""
		if req.Subscription.ExpirationTime != nil {
			grpcReq.ExpirationTime = *req.Subscription.ExpirationTime
		}
		grpcReq.P256Dh = req.Subscription.Keys.P256Dh
		grpcReq.Auth = req.Subscription.Keys.Auth
	}

	// Führe den gRPC-Call aus
	createPushSubscriptionResponse, err := n.pushSubscriptionService.CreatePushSubscription(ctx, grpcReq)
	if err != nil {
		returnErr := goerrors.InternalServerError
		if status.Code(err) == codes.NotFound {
			returnErr = goerrors.NotificationNotFound
		}
		n.logger.Infof("Error in upstream call n.pushSubscriptionService.CreatePushSubscription: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	response := schema.CreatePushSubscriptionResponse{
		SubscriptionID: createPushSubscriptionResponse.SubscriptionId,
	}
	c.JSON(201, response)
}

func (n *NotificationHandler) GetNotifications(c *gin.Context) {
	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	notifications, err := n.notificationService.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{})

	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		}

		n.logger.Infof("Error in upstream call n.notificationService.GetNotification: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}

	c.JSON(200, dto.TransformNotificationProtoToDTO(notifications))
}

func (n *NotificationHandler) DeleteNotification(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := n.notificationService.DeleteNotification(ctx, &notificationv1.DeleteNotificationRequest{
		NotificationId: c.Param("notificationId"),
	})

	if err != nil {
		returnErr := goerrors.InternalServerError
		if status.Code(err) == codes.NotFound {
			returnErr = goerrors.NotificationNotFound
		}
		n.logger.Infof("Error in upstream call n.notificationService.DeleteNotification: %v", err)
		c.JSON(returnErr.HttpStatus, returnErr)
		return
	}
	c.JSON(204, notificationv1.DeleteNotificationResponse{})
}
