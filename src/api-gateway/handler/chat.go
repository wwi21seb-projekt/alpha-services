package handler

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler/ws"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
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

	// We use this as a workaround to handle auth, since browsers still
	// don't support custom headers in websocket connections. Since the
	// appended protocol would break the upgrade, we mutate the request
	// back to its original state when we're done.
	token := ""
	headerKey := http.CanonicalHeaderKey("Sec-WebSocket-Protocol")
	for _, value := range c.Request.Header[headerKey] {
		if strings.HasPrefix(value, "ey") {
			token = value
		}
	}

	if token == "" {
		log.Error("No authorization header provided")
		c.JSON(http.StatusUnauthorized, schema.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}

	username, err := ch.jwtManager.Verify(token)
	if err != nil {
		log.Error("Failed to verify token: ", err)
		c.JSON(http.StatusUnauthorized, schema.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}

	chatId := c.Query("chatId")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(string(keys.SubjectKey), username))
	log.Info("Preparing chat stream...")

	if _, err = ch.chatServiceClient.PrepareChatStream(ctx, &pb.PrepareChatStreamRequest{
		ChatId: chatId,
	}); err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			log.Warn("Chat not found")
			returnErr = goerrors.ChatNotFound
		} else if code == codes.AlreadyExists {
			log.Warn("User is already connected to this chat")
			returnErr = &goerrors.CustomError{
				HttpStatus: http.StatusTeapot,
				Title:      "AlreadyConnected",
				Code:       "ERR-999",
				Message:    "User is already connected to this chat",
			}
		}

		log.Error("Error in ch.chatServiceClient.PrepareChatStream: ", err)
		c.JSON(returnErr.HttpStatus, schema.ErrorDTO{
			Error: returnErr,
		})
		return
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "chatId", chatId)
	log.Info("Creating chat stream...")
	stream, err := ch.chatServiceClient.ChatStream(ctx)
	if err != nil {
		status := status.Convert(err)
		returnErr := goerrors.InternalServerError

		if status.Code() == codes.FailedPrecondition {
			log.Error("Chat stream not prepared")
			returnErr = goerrors.BadRequest
		}

		log.Error("Failed to create chat stream: ", err)
		c.JSON(returnErr.HttpStatus, schema.ErrorDTO{
			Error: returnErr,
		})
		return
	}
	defer stream.CloseSend()

	// We need to pass the token in the websocket connection into the subprotocols, because
	// the client expects it there. This is a consequence of the browser not allowing custom
	// headers in websocket connections.
	ch.upgrader.Subprotocols = []string{token}

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
	var wg sync.WaitGroup
	wg.Add(3)

	go client.WritePump(&wg)
	go client.ReadPump(&wg)
	go client.GrpcReceivePump(&wg)
	log.Info("Pumps started, waiting for client to disconnect...")

	<-client.Disconnect
	log.Info("ChatHandler: Client disconnected, cleaning up...")
	ch.hub.Unregister <- client

	// Wait for all pumps to finish their cleanup before returning
	wg.Wait()
	log.Info("ChatHandler: Client cleanup finished")
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
		} else if code == codes.InvalidArgument {
			returnErr = goerrors.BadRequest
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
