package handler

import (
	"context"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	chatv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/chat/v1"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/handler/ws"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/helper"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/manager"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/middleware"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
	logger            *zap.SugaredLogger
	tracer            trace.Tracer
	jwtManager        manager.JWTManager
	upgrader          websocket.Upgrader
	chatServiceClient chatv1.ChatServiceClient
	hub               *ws.Hub
}

func NewChatHandler(logger *zap.SugaredLogger, jwtManager manager.JWTManager, chatClient chatv1.ChatServiceClient, hub *ws.Hub) ChatHdlr {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return &ChatHandler{
		logger:            logger,
		tracer:            otel.GetTracerProvider().Tracer("chat-handler"),
		jwtManager:        jwtManager,
		chatServiceClient: chatClient,
		upgrader:          upgrader,
		hub:               hub,
	}
}

// Chat implements ChatHdlr.
func (ch *ChatHandler) Chat(c *gin.Context) {
	ch.logger.Info("ChatHandler: Chat endpoint called, checking authorization...")
	_, checkAuthSpan := ch.tracer.Start(c.Request.Context(), "CheckAuthorization")

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
		checkAuthSpan.AddEvent("No authorization header provided")
		checkAuthSpan.End()
		ch.logger.Error("No authorization header provided")
		c.JSON(http.StatusUnauthorized, dto.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}

	username, err := ch.jwtManager.Verify(token)
	if err != nil {
		checkAuthSpan.AddEvent("Failed to verify token")
		checkAuthSpan.End()
		ch.logger.Error("Failed to verify token: ", err)
		c.JSON(http.StatusUnauthorized, dto.ErrorDTO{
			Error: goerrors.Unauthorized,
		})
		return
	}
	checkAuthSpan.End()

	chatId := c.Query("chatId")
	ctx := metadata.NewOutgoingContext(c.Request.Context(), metadata.Pairs(string(keys.SubjectKey), username))
	ch.logger.Info("ChatHandler: Preparing chat stream...")

	if _, err = ch.chatServiceClient.PrepareChatStream(ctx, &chatv1.PrepareChatStreamRequest{
		ChatId: chatId,
	}); err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			ch.logger.Warn("Chat not found")
			returnErr = goerrors.ChatNotFound
		}

		ch.logger.Error("Error in ch.chatServiceClient.PrepareChatStream: ", err)
		c.JSON(returnErr.HttpStatus, dto.ErrorDTO{
			Error: returnErr,
		})
		return
	}

	ctx = metadata.AppendToOutgoingContext(ctx, string(keys.ChatIDKey), chatId)
	ch.logger.Info("ChatHandler: Creating chat stream...")
	stream, err := ch.chatServiceClient.ChatMessage(ctx)
	if err != nil {
		status := status.Convert(err)
		returnErr := goerrors.InternalServerError

		if status.Code() == codes.FailedPrecondition {
			ch.logger.Error("Chat stream not prepared")
			returnErr = goerrors.BadRequest
		}

		ch.logger.Error("Failed to create chat stream: ", err)
		c.JSON(returnErr.HttpStatus, dto.ErrorDTO{
			Error: returnErr,
		})
		return
	}
	defer stream.CloseSend()

	// We need to pass the token in the websocket connection into the subprotocols, because
	// the client expects it there. This is a consequence of the browser not allowing custom
	// headers in websocket connections.
	_, upgradeSpan := ch.tracer.Start(ctx, "UpgradeToWebsocket")
	ch.upgrader.Subprotocols = []string{token}

	// Upgrade to websocket
	ch.logger.Info("ChatHandler: Upgrading to websocket...")
	conn, err := ch.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		upgradeSpan.AddEvent("Failed to upgrade to websocket")
		upgradeSpan.End()
		ch.logger.Error("Failed to upgrade to websocket: ", err)
		c.JSON(http.StatusInternalServerError, dto.ErrorDTO{
			Error: goerrors.InternalServerError,
		})
		return
	}
	upgradeSpan.End()
	defer conn.Close()

	// Register client in hub
	client := ws.NewClient(ch.logger, ch.hub, conn, stream, username)
	ch.hub.Register <- client
	ch.logger.Info("ChatHandler: Client registered, starting pumps...")
	// We wrap the pumps in a WaitGroup to ensure that we wait for all pumps to finish
	// before returning from this function. This is necessary to ensure that we don't
	// close the connection before all pumps have finished their cleanup.
	var wg sync.WaitGroup
	wg.Add(3)

	go client.WritePump(&wg)
	go client.ReadPump(&wg)
	go client.GrpcReceivePump(&wg)
	ch.logger.Info("ChatHandler: Pumps started, waiting for client to disconnect...")

	<-client.Disconnect
	ch.logger.Info("ChatHandler: Client disconnected, cleaning up...")
	ch.hub.Unregister <- client

	// Wait for all pumps to finish and then return
	wg.Wait()
	ch.logger.Info("ChatHandler: Client cleanup finished")
}

// CreateChat implements ChatHdlr.
func (ch *ChatHandler) CreateChat(c *gin.Context) {
	req := c.MustGet(middleware.SanitizedPayloadKey.String()).(*schema.CreateChatRequest)

	// Get outgoing context
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	// Call chat service
	resp, err := ch.chatServiceClient.CreateChat(ctx, &chatv1.CreateChatRequest{
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

		ch.logger.Error("Error in ch.chatServiceClient.CreateChat: ", err)
		c.JSON(returnErr.HttpStatus, dto.ErrorDTO{
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

	resp, err := ch.chatServiceClient.GetChat(ctx, &chatv1.GetChatRequest{
		ChatId: chatID,
		Pagination: &commonv1.PaginationRequest{
			PageToken: strconv.FormatInt(offset, 10),
			PageSize:  limit,
		},
	})
	if err != nil {
		code := status.Code(err)
		returnErr := goerrors.InternalServerError

		if code == codes.NotFound {
			returnErr = goerrors.ChatNotFound
		}

		ch.logger.Error("Error in ch.chatServiceClient.GetChat: ", err)
		c.JSON(http.StatusInternalServerError, dto.ErrorDTO{
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
			Records: resp.Pagination.GetTotalSize(),
			Offset:  offset,
			Limit:   limit,
		},
	})
}

// GetChats implements ChatHdlr.
func (ch *ChatHandler) GetChats(c *gin.Context) {
	ctx := c.MustGet(middleware.GRPCMetadataKey).(context.Context)

	resp, err := ch.chatServiceClient.ListChats(ctx, &chatv1.ListChatsRequest{})
	if err != nil {
		ch.logger.Error("Error in ch.chatServiceClient.ListChats: ", err)
		c.JSON(http.StatusInternalServerError, dto.ErrorDTO{
			Error: goerrors.InternalServerError,
		})
		return
	}

	chats := make([]schema.ChatInformation, 0, len(resp.Chats))
	for _, chat := range resp.Chats {
		chat := schema.ChatInformation{
			ChatID: chat.GetId(),
			User: schema.Author{
				Username: chat.GetUser().GetUsername(),
				Nickname: chat.GetUser().GetNickname(),
				Picture: &schema.Picture{
					Url:    chat.GetUser().GetPicture().GetUrl(),
					Width:  chat.GetUser().GetPicture().GetWidth(),
					Height: chat.GetUser().GetPicture().GetHeight(),
				},
			},
		}

		chats = append(chats, chat)
	}

	c.JSON(http.StatusOK, &schema.GetChatsResponse{
		Chats: chats,
	})
}
