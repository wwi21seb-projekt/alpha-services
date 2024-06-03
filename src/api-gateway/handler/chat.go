package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler/ws"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ChatHdlr interface {
	CreateChat(c *gin.Context) // POST /chats
	GetChats(c *gin.Context)   // GET /chats
	GetChat(c *gin.Context)    // GET /chats/:chatId
	Chat(c *gin.Context)       // GET /chat (websocket)
}

type ChatHandler struct {
	jwtManager        manager.JWTManager
	upgrader          websocket.Upgrader
	chatServiceClient pb.ChatServiceClient
	hub               *ws.Hub
}

func NewChatHandler(jwtManager manager.JWTManager, chatClient pb.ChatServiceClient, hub *ws.Hub) ChatHdlr {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	return &ChatHandler{
		jwtManager:        jwtManager,
		chatServiceClient: chatClient,
		upgrader:          upgrader,
		hub:               hub,
	}
}

// Chat implements ChatHdlr.
func (ch *ChatHandler) Chat(c *gin.Context) {
	log.Info("Chat endpoint called, checking authorization...")

	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		log.Error("No authorization header provided")
		c.JSON(http.StatusUnauthorized, schema.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}

	token := authHeader[7:]
	username, err := ch.jwtManager.Verify(token)
	if err != nil {
		log.Error("Failed to verify token: ", err)
		c.JSON(http.StatusUnauthorized, schema.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}

	chatId := c.Query("chatId")
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)
	log.Info("Preparing chat stream...")

	if _, err = ch.chatServiceClient.PrepareChatStream(ctx, &pb.PrepareChatStreamRequest{
		ChatId: chatId,
	}); err != nil {
		code := status.Code(err)

		if code == codes.NotFound {
			c.JSON(http.StatusNotFound, schema.ErrorDTO{
				Error: goerrors.ChatNotFound,
			})
			return
		}

		log.Error("Error in ch.chatServiceClient.PrepareChatStream: ", err)
		c.JSON(http.StatusInternalServerError, schema.ErrorDTO{
			Error: goerrors.InternalServerError,
		})
		return
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "chatId", chatId)
	log.Info("Creating chat stream...")
	stream, err := ch.chatServiceClient.ChatStream(ctx)
	if err != nil {
		log.Error("Failed to create chat stream: ", err)
		c.JSON(http.StatusInternalServerError, schema.ErrorDTO{
			Error: goerrors.InternalServerError,
		})
		return
	}
	defer stream.CloseSend()

	// Upgrade to websocket
	log.Info("Upgrading to websocket...")
	conn, err := ch.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("Failed to upgrade to websocket: ", err)
		c.JSON(http.StatusUnauthorized, schema.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}
	defer conn.Close()

	// Register client in hub
	client := ws.NewClient(ch.hub, conn, stream, username)
	ch.hub.Register <- client
	log.Info("Client registered, starting pumps...")

	go client.WritePump()
	go client.ReadPump()
	go client.GrpcReceivePump()

	log.Info("Pumps started, waiting for client to disconnect...")
	<-client.Disconnect

	log.Info("Client disconnected, unregistering...")
	ch.hub.Unregister <- client
}

// CreateChat implements ChatHdlr.
func (ch *ChatHandler) CreateChat(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.CreateChatRequest)

	// Get outgoing context
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	// Call chat service
	resp, err := ch.chatServiceClient.CreateChat(ctx, &pb.CreateChatRequest{
		Username: req.Username,
		Message:  req.Content,
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.UserNotFound
		} else if code == codes.AlreadyExists {
			returnErr = goerrors.ChatAlreadyExists
		}

		log.Error("Error in ch.chatServiceClient.CreateChat: ", err)
		c.JSON(returnErr.HttpStatus, schema.ErrorDTO{
			Error: returnErr,
		})
		return
	}

	c.JSON(http.StatusCreated, schema.CreateChatResponse{
		ChatID: resp.GetChatId(),
		Message: schema.Message{
			Username:     resp.GetMessage().GetUsername(),
			Content:      resp.GetMessage().GetMessage(),
			CreationDate: resp.GetMessage().GetCreatedAt(),
		},
	})
}

// GetChat implements ChatHdlr.
func (ch *ChatHandler) GetChat(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)
	chatID := c.Param("chatId")
	offset, limit := helper.ExtractPaginationFromContext(c)

	resp, err := ch.chatServiceClient.GetChat(ctx, &pb.GetChatRequest{
		ChatId: chatID,
		Pagination: &pbCommon.PaginationRequest{
			Offset: int32(offset),
			Limit:  int32(limit),
		},
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.ChatNotFound
		}

		log.Error("Error in ch.chatServiceClient.GetChat: ", err)
		c.JSON(http.StatusInternalServerError, schema.ErrorDTO{
			Error: returnErr,
		})
		return
	}

	messages := make([]schema.Message, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		messages = append(messages, schema.Message{
			Username:     msg.Username,
			Content:      msg.Message,
			CreationDate: msg.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, schema.GetChatResponse{
		Messages: messages,
		Pagination: schema.PaginationResponse{
			Records: resp.Pagination.Records,
			Offset:  resp.Pagination.Offset,
			Limit:   resp.Pagination.Limit,
		},
	})
}

// GetChats implements ChatHdlr.
func (ch *ChatHandler) GetChats(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	resp, err := ch.chatServiceClient.ListChats(ctx, &pbCommon.Empty{})
	if err != nil {
		log.Error("Error in ch.chatServiceClient.ListChats: ", err)
		c.JSON(http.StatusInternalServerError, schema.ErrorDTO{
			Error: goerrors.InternalServerError,
		})
		return
	}

	chats := make([]schema.ChatInformation, 0, len(resp.Chats))
	for _, chat := range resp.Chats {
		chat := schema.ChatInformation{
			ChatID: chat.GetId(),
			User: schema.Author{
				Username:          chat.GetUser().GetUsername(),
				Nickname:          chat.GetUser().GetNickname(),
				ProfilePictureUrl: chat.GetUser().GetProfilePictureUrl(),
			},
		}

		chats = append(chats, chat)
	}

	c.JSON(http.StatusOK, &schema.GetChatsResponse{
		Chats: chats,
	})
}
