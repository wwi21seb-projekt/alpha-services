package handler

import (
	"encoding/base64"
	"fmt"
	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/wwi21seb-projekt/errors-go/goerrors"
)

type ImageHdlr interface {
	GetImage(c *gin.Context) // GET /images
}

type imageHandler struct {
	logger      *zap.SugaredLogger
	tracer      trace.Tracer
	imageClient imagev1.ImageServiceClient
}

func NewImageHandler(logger *zap.SugaredLogger, imageServiceClient imagev1.ImageServiceClient) ImageHdlr {
	return &imageHandler{
		logger:      logger,
		tracer:      otel.GetTracerProvider().Tracer("image-handler"),
		imageClient: imageServiceClient,
	}
}

func (i *imageHandler) GetImage(c *gin.Context) {
	// Get outgoing context from metadata
	ctx := c.Request.Context()

	// Get image from request
	image := c.Query("image")

	// Get image from storage
	i.logger.Infof("Getting image %s", image)
	imageResponse, err := i.imageClient.GetImage(ctx, &imagev1.GetImageRequest{
		Name: image,
	})
	if err != nil {
		i.logger.Errorf("Error in upstream call i.imageClient.GetImage: %v", err)
		c.JSON(500, goerrors.InternalServerError)
		return
	}

	// Convert base64 string to image
	contentType := fmt.Sprintf("image/%s", imageResponse.GetType())
	imageBytes, err := decodeBase64Image(i.logger, imageResponse.GetImage())
	if err != nil {
		i.logger.Errorf("Failed to decode base64 image: %v", err)
		c.JSON(500, goerrors.InternalServerError)
		return
	}

	// Return image
	c.Data(200, contentType, imageBytes)
}

func decodeBase64Image(logger *zap.SugaredLogger, base64Image string) ([]byte, error) {
	// Decode the base64 image into bytes
	imageBytes, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		logger.Errorf("Failed to decode base64 image: %v", err)
		return nil, err
	}
	return imageBytes, nil
}
