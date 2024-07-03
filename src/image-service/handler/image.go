package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"go.uber.org/zap"
)

const (
	MaxImageSize = 3 * 1024 * 1024      // 3 MB in bytes
	ImageDir     = "/serveralpha/data/" // Directory to store images
)

type imageService struct {
	logger *zap.SugaredLogger
	imagev1.UnimplementedImageServiceServer
}

func NewImageServiceServer(logger *zap.SugaredLogger) imagev1.ImageServiceServer {
	return &imageService{
		logger: logger,
	}
}

func (s *imageService) UploadImage(ctx context.Context, request *imagev1.UploadImageRequest) (*imagev1.UploadImageResponse, error) {
	// Decode the base64 image into bytes
	imageBytes, err := s.decodeBase64Image(request.GetImage())
	if err != nil {
		s.logger.Error("Failed to decode base64 image", zap.Error(err))
		return nil, err
	}

	// Validate the image type to be webp, jpeg or png
	im, imageType, err := s.validateImage(imageBytes)
	if err != nil {
		s.logger.Error("Failed to validate image", zap.Error(err))
		return nil, err
	}

	// Upload the image to the storage
	filename := s.uploadImage(imageBytes, imageType, request.GetName())

	response := &imagev1.UploadImageResponse{
		Url:    filename,
		Width:  int32(im.Bounds().Dx()),
		Height: int32(im.Bounds().Dy()),
	}

	return response, nil
}

func (s *imageService) GetImage(ctx context.Context, request *imagev1.GetImageRequest) (*imagev1.GetImageResponse, error) {
	// Read the image from the file
	filePath := filepath.Join(ImageDir, request.GetName())
	imageBytes, err := os.ReadFile(filePath)
	if err != nil {
		s.logger.Error("Failed to read image from file")
		return nil, status.Errorf(codes.NotFound, "image not found %s", request.GetName())
	}

	// Encode the image to base64
	base64Image := encodeBase64Image(imageBytes)
	return &imagev1.GetImageResponse{
		Image: base64Image,
		Type:  filepath.Ext(filePath)[1:],
	}, nil
}

func (s *imageService) uploadImage(imageBytes []byte, imageType string, imageContext string) string {
	s.logger.Info("Uploading image...")
	fileName := fmt.Sprintf("%s.%s", imageContext, imageType)
	filePath := filepath.Join(ImageDir, fileName)

	s.logger.Infow("Image path", "path", filePath)
	s.logger.Infow("Image name", "name", fileName)

	// Validate the image directory
	if _, err := os.Stat(ImageDir); os.IsNotExist(err) {
		s.logger.Info("Creating image directory...")
		err := os.MkdirAll(ImageDir, 0755)
		if err != nil {
			s.logger.Error("Failed to create image directory", zap.Error(err))
			return ""
		}
	}

	// Write the image to the file and overwrite if it already exists
	err := os.WriteFile(filePath, imageBytes, 0644)
	if err != nil {
		s.logger.Error("Failed to write image to file", zap.Error(err))
		return ""
	}
	s.logger.Infow("Image uploaded", "path", filePath)

	return fileName
}

func (s *imageService) validateImage(imageBytes []byte) (image.Image, string, error) {
	s.logger.Info("Checking image type...")
	img, imageType, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		s.logger.Error("Failed to decode image", zap.Error(err))
		return nil, "", status.Errorf(codes.InvalidArgument, "failed to decode image")
	}

	switch imageType {
	case "jpeg", "png", "webp":
		// Check if the image size is within the limit
		if len(imageBytes) > MaxImageSize {
			s.logger.Error("Image size exceeds the limit")
			return nil, "", status.Errorf(codes.InvalidArgument, "image size exceeds the limit with %d bytes", len(imageBytes))
		}

		return img, imageType, nil
	default:
		s.logger.Error("Invalid image type")
		return nil, "", status.Errorf(codes.InvalidArgument, "invalid image type %s", imageType)
	}
}

func (s *imageService) decodeBase64Image(base64Image string) ([]byte, error) {
	s.logger.Infow("Decoding base64 image", "image", base64Image)
	base64str := base64Image

	if strings.HasPrefix(base64Image, "data:image") {
		parts := strings.SplitN(base64Image, ",", 2)

		if len(parts) != 2 {
			s.logger.Error("Invalid base64 image")
			return nil, status.Errorf(codes.InvalidArgument, "invalid base64 image")
		}

		base64str = parts[1]
	}

	img, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		s.logger.Error("Failed to decode base64 image", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, "failed to decode base64 image")
	}

	return img, nil
}

func encodeBase64Image(image []byte) string {
	return base64.StdEncoding.EncodeToString(image)
}
