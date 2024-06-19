package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	pb "github.com/wwi21seb-projekt/alpha-shared/proto/image"
	"go.uber.org/zap"
)

const (
	MaxImageSize = 3 * 1024 * 1024       // 3 MB in bytes
	ImageDir     = "/data/image-service" // Directory to store images
)

type imageService struct {
	logger *zap.SugaredLogger
	pb.UnimplementedImageServiceServer
}

func NewImageServiceServer(logger *zap.SugaredLogger) pb.ImageServiceServer {
	return &imageService{
		logger: logger,
	}
}

func (s *imageService) UploadImage(ctx context.Context, request *pb.UploadImageRequest) (*pb.UploadImageResponse, error) {
	// Decode the base64 image into bytes
	imageBytes, err := decodeBase64Image(s.logger, request.GetImage())
	if err != nil {
		s.logger.Error("Failed to decode base64 image", zap.Error(err))
		return nil, err
	}

	// Validate the image type to be webp, jpeg or png
	image, imageType, err := validateImage(s.logger, imageBytes)
	if err != nil {
		s.logger.Error("Failed to validate image", zap.Error(err))
		return nil, err
	}

	// Upload the image to the storage
	filename := uploadImage(s.logger, imageBytes, imageType, request.GetContextString())

	response := &pb.UploadImageResponse{
		Url:    filename,
		Width:  int32(image.Bounds().Dx()),
		Height: int32(image.Bounds().Dy()),
	}

	return response, nil
}

func (s *imageService) GetImage(ctx context.Context, request *pb.GetImageRequest) (*pb.Image, error) {
	// Read the image from the file
	filePath := filepath.Join(ImageDir, request.GetImageName())
	imageBytes, err := os.ReadFile(filePath)
	if err != nil {
		s.logger.Error("Failed to read image from file", zap.Error(err))
		return nil, err
	}

	// Encode the image to base64
	base64Image := encodeBase64Image(imageBytes)
	return &pb.Image{
		Base64Image: base64Image,
		ImageType:   filepath.Ext(filePath)[1:],
	}, nil
}

func uploadImage(logger *zap.SugaredLogger, imageBytes []byte, imageType string, imageContext string) string {
	logger.Info("Uploading image...")
	fileName := fmt.Sprintf("%s.%s", imageContext, imageType)
	filePath := filepath.Join(ImageDir, fileName)

	// Validate the image directory
	if _, err := os.Stat(ImageDir); os.IsNotExist(err) {
		logger.Info("Creating image directory...")
		err := os.MkdirAll(ImageDir, 0755)
		if err != nil {
			logger.Error("Failed to create image directory", zap.Error(err))
			return ""
		}
	}

	// Write the image to the file and overwrite if it already exists
	err := os.WriteFile(filePath, imageBytes, 0644)
	if err != nil {
		logger.Error("Failed to write image to file", zap.Error(err))
		return ""
	}

	return fileName
}

func validateImage(logger *zap.SugaredLogger, imageBytes []byte) (image.Image, string, error) {
	logger.Info("Checking image type...")
	image, imageType, err := image.Decode(bytes.NewReader(imageBytes))

	switch imageType {
	case "jpeg", "png", "webp":
		// Check if the image size is within the limit
		if len(imageBytes) > MaxImageSize {
			logger.Error("Image size exceeds the limit")
			return nil, "", errors.New("image size exceeds the limit")
		}

		return image, imageType, nil
	default:
		logger.Error("Invalid image type")
		return nil, "", errors.Join(errors.New("invalid image type"), err)
	}
}

func decodeBase64Image(logger *zap.SugaredLogger, base64Image string) ([]byte, error) {
	logger.Info("Decoding base64 image...")
	base64str := base64Image

	if strings.HasPrefix(base64Image, "data:image") {
		parts := strings.SplitN(base64Image, ",", 2)

		if len(parts) != 2 {
			logger.Error("Invalid base64 image")
			return nil, errors.New("invalid base64 image")
		}

		base64str = parts[1]
	}

	image, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		logger.Error("Failed to decode base64 image", zap.Error(err))
		return nil, err
	}

	return image, nil
}

func encodeBase64Image(image []byte) string {
	return base64.StdEncoding.EncodeToString(image)
}
