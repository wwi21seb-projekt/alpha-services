package middleware

import (
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"google.golang.org/grpc/metadata"

	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
)

var unauthorizedError = &schema.ErrorDTO{Error: goerrors.Unauthorized}
var GRPCMetadataKey = "grpc-metadata" // to be added to shared keys

func SetClaimsMiddleware(jwtManager manager.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader("Authorization")
		if authorizationHeader == "" {
			log.Error("Authorization header is missing")
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		authorizationHeaderParts := strings.Split(authorizationHeader, " ")
		if len(authorizationHeaderParts) != 2 {
			log.Error("Authorization header is invalid")
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		tokenString := authorizationHeaderParts[1]
		username, err := jwtManager.Verify(tokenString)
		if err != nil {
			log.Errorf("Error in jwtManager.Verify: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		// Set the username and token in the context
		c.Set(string(keys.SubjectKey), username)
		c.Set(string(keys.TokenKey), tokenString)
		// Create initial gRPC metadata with the username
		ctx := metadata.AppendToOutgoingContext(c.Request.Context(), string(keys.SubjectKey), username)
		c.Set(string(GRPCMetadataKey), ctx)
		c.Next()
	}

}
