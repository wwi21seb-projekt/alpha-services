package api_gateway_vanilla

import (
	"github.com/gin-gonic/gin"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway-vanilla/handler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

type server struct{}

func main() {
	// Set up a connection to the gRPC server
	conn, err := grpc.Dial("your_grpc_server_address:your_grpc_server_port", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	// Create a new PostHandler
	postHandler := handler.NewPostHandler(conn)

	// Set up Gin router
	r := gin.Default()

	// Define your routes
	r.POST("/posts", postHandler.CreatePost)
	r.GET("/feed", postHandler.GetFeed)

	// Run the Gin server
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
