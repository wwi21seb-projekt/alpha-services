package handler

import (
	"github.com/wwi21seb-projekt/alpha-shared/db"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type interactionService struct {
	logger        *zap.SugaredLogger
	tracer        trace.Tracer
	db            *db.DB
	profileClient pbUser.UserServiceClient
	pb.UnimplementedInteractionServiceServer
}

func NewInteractionService(logger *zap.SugaredLogger, db *db.DB, profileClient pbUser.UserServiceClient) pb.InteractionServiceServer {
	return &interactionService{
		logger:        logger,
		tracer:        otel.GetTracerProvider().Tracer("post-service"),
		db:            db,
		profileClient: profileClient,
	}
}
