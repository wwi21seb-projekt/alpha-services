package main

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"go-micro.dev/v4"
	"go-micro.dev/v4/auth"
	"go-micro.dev/v4/metadata"
	"go-micro.dev/v4/server"
	"strings"

	postpb "github.com/wwi21seb-projekt/alpha-services/api-gateway/proto/post-service"
)

var rules = []*auth.Rule{
	{
		Resource: &auth.Resource{
			Type:     "service",
			Name:     "post-service",
			Endpoint: "Health.Check",
		},
		Access: auth.AccessGranted,
	},
}

func main() {
	srv := micro.NewService()

	srv.Init(
		micro.Name("api-gateway"),
		micro.Version("latest"),
		micro.Auth(auth.DefaultAuth),
		micro.WrapHandler(authWrapper(srv)),
	)

	// Create client stub for post-service
	postService := postpb.NewHealthService("post-service", srv.Client())

	// Expose HTTP endpoint with go-micro server
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		// Call post-service
		res, err := postService.Check(context.Background(), &postpb.HealthCheckRequest{})
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, res)
	})

	// Run the service asynchronously
	go func() {
		if err := srv.Run(); err != nil {
			return
		}
	}()

	err := r.Run(":8080")
	if err != nil {
		return
	}
}

// authWrapper is a handler wrapper for auth validation
func authWrapper(service micro.Service) server.HandlerWrapper {
	return func(h server.HandlerFunc) server.HandlerFunc {
		return func(ctx context.Context, req server.Request, rsp interface{}) error {
			// Fetch metadata from context (request headers).
			md, b := metadata.FromContext(ctx)
			if !b {
				return errors.New("no metadata found")
			}

			// Get auth header.
			authHeader, ok := md["Authorization"]
			if !ok || !strings.HasPrefix(authHeader, auth.BearerScheme) {
				return errors.New("no auth token provided")
			}

			// Extract auth token.
			token := strings.TrimPrefix(authHeader, auth.BearerScheme)

			// Extract account from token.
			a := service.Options().Auth
			acc, err := a.Inspect(token)
			if err != nil {
				return errors.New("auth token invalid")
			}

			// Create resource for current endpoint from request headers.
			currentResource := auth.Resource{
				Type:     "service",
				Name:     md["Micro-Service"],
				Endpoint: md["Micro-Endpoint"],
			}

			// Verify if account has access.
			if err := auth.Verify(rules, acc, &currentResource); err != nil {
				return errors.New("no access")
			}

			return h(ctx, req, rsp)
		}
	}
}
