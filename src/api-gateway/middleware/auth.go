package middleware

import (
	"net/http"
	"strings"

	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
)

var jwtSecret = []byte("your_secret_key")
var unauthorizedError = &schema.ErrorDTO{Error: goerrors.Unauthorized}

func SetClaimsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			c.Next() // No token, continue
			return
		}

		// Bearer token check
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
			return
		}

		// Store the claims in the context
		c.Set("claims", claims)
		c.Set("token", tokenString)

		c.Next()
	}

}

// RequireAuthMiddleware is a middleware that checks if the request is authenticated
func RequireAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("claims"); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
