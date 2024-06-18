package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbNotification "github.com/wwi21seb-projekt/alpha-shared/proto/notification"
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
	notificationService     pbNotification.NotificationServiceClient
	pushSubscriptionService pbNotification.PushServiceClient
}

func NewNotificationHandler(logger *zap.SugaredLogger, notificationClient pbNotification.NotificationServiceClient, pushSubscriptionClient pbNotification.PushServiceClient) NotificationHdlr {
	return &NotificationHandler{
		logger:                  logger,
		notificationService:     notificationClient,
		pushSubscriptionService: pushSubscriptionClient,
	}
}

func (n *NotificationHandler) GetPublicKey(c *gin.Context) {
	// Get outgoing context from metadata
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	publicKey, err := n.pushSubscriptionService.GetPublicKey(ctx, &pbCommon.Empty{})

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
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	createPushSubscriptionResponse, err := n.pushSubscriptionService.CreatePushSubscription(ctx, &pbNotification.CreatePushSubscriptionRequest{
		// Convert string to enum
		Type: func(s string) pbNotification.PushSubscriptionType {
			if val, ok := pbNotification.PushSubscriptionType_value[s]; ok {
				return pbNotification.PushSubscriptionType(val)
			}
			return pbNotification.PushSubscriptionType_WEB // web is returned by defualt
		}(c.Param("type")),
		Token:          c.Param("token"),
		Endpoint:       c.Param("endpoint"),
		ExpirationTime: c.Param("expirationTime"),
		P256Dh:         c.Param("p256dh"),
		Auth:           c.Param("auth"),
	})

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

	notifications, err := n.notificationService.GetNotifications(ctx, &pbCommon.Empty{})

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

	response := schema.GetNotificationsResponse{}
	for _, notification := range notifications.Notifications {
		responseUser := schema.Author{
			Username:          notification.User.Username,
			Nickname:          notification.User.Nickname,
			ProfilePictureUrl: notification.User.ProfilePictureUrl,
		}
		responseNotification := schema.Notification{
			NotificationID:   notification.NotificationId,
			Timestamp:        notification.Timestamp,
			NotificationType: notification.NotficationType.String(),
			User:             responseUser,
		}
		response.Records = append(response.Records, responseNotification)
	}
	c.JSON(200, response)
}

func (n *NotificationHandler) DeleteNotification(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	_, err := n.notificationService.DeleteNotification(ctx, &pbNotification.DeleteNotificationRequest{
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
	c.JSON(204, pbCommon.Empty{})
}
