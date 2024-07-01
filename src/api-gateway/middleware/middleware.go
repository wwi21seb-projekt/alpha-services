package middleware

import (
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"go.uber.org/zap"
)

type Middleware struct {
	logger     *zap.SugaredLogger
	validator  *Validator
	jwtManager manager.JWTManager
}

func NewMiddleware(logger *zap.SugaredLogger, jwtManager manager.JWTManager) *Middleware {
	return &Middleware{
		logger:     logger,
		validator:  NewValidator(),
		jwtManager: jwtManager,
	}
}
