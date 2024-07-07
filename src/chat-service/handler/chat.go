package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	chatv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/chat/v1"
	commonv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/common/v1"
	notificationv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/notification/v1"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type chatService struct {
	logger     *zap.SugaredLogger
	tracer     trace.Tracer
	db         *db.DB
	userClient userv1.UserServiceClient
	notificationClient notificationv1.NotificationServiceClient
	streams    map[string]*chatStream
	mu         sync.RWMutex
	chatv1.UnimplementedChatServiceServer
}

func NewChatService(logger *zap.SugaredLogger, db *db.DB, userClient userv1.UserServiceClient, notificationClient notificationv1.NotificationServiceClient) chatv1.ChatServiceServer {
	return &chatService{
		logger:     logger,
		tracer:     otel.GetTracerProvider().Tracer("chat-service"),
		db:         db,
		userClient: userClient,
		notificationClient: notificationClient,
		streams:    make(map[string]*chatStream),
	}
}

// chatStream is a struct that holds all the connections for a chat
// it's used in the streamMap and accessed by the chatId
type chatStream struct {
	connections []*openConnection
}

// openConnection is a struct that holds the connection to the client
// it contains metadata about the connection and the connection itself
type openConnection struct {
	// streams is a list of all the streams that are connected to the chat
	// from the same user. we allow multiple connections from the same user
	// to allow the user to connect from multiple devices. but we need to
	// save all the references to the streams to be able to send messages
	// to all of them
	streams []chatv1.ChatService_ChatMessageServer
	// active is used to determine if the connection is active or not. it's used to
	// determine if the connection was prepared but not yet connected
	active bool
	// username is the username of the user that is connected to the chat
	username string
}

// CreateChat implements serveralpha.ChatServiceServer.
func (cs *chatService) CreateChat(ctx context.Context, req *chatv1.CreateChatRequest) (*chatv1.CreateChatResponse, error) {
	conn, err := cs.db.Acquire(ctx)
	if err != nil {
		cs.logger.Errorf("Error in cs.db.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	tx, err := cs.db.BeginTx(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer cs.db.RollbackTx(ctx, tx)

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	chatId, messageId := uuid.New(), uuid.New()
	createdAt := time.Now()

	// Check if it's a self-chat
	if username == req.GetUsername() {
		return nil, status.Error(codes.InvalidArgument, "cannot create chat with yourself")
	}

	// Sort usernames to avoid duplicate chats
	user1, user2, constraintName := username, req.GetUsername(), "user2_fk"
	if user1 > user2 {
		user1, user2, constraintName = user2, user1, "user1_fk"
	}

	chatsQuery, chatsArgs, _ := psql.Insert("chats").
		Columns("chat_id", "created_at", "user1_name", "user2_name").
		Values(chatId, createdAt, user1, user2).
		ToSql()

	messageQuery, messageArgs, _ := psql.Insert("messages").
		Columns("chat_id", "message_id", "sender_name", "content", "created_at").
		Values(chatId, messageId, username, req.GetMessage(), createdAt).
		ToSql()

	batch := pgx.Batch{}
	batch.Queue(chatsQuery, chatsArgs...)
	batch.Queue(messageQuery, messageArgs...)
	cs.logger.Info("Trying to create chat and initial message...")
	br := tx.SendBatch(ctx, &batch)
	defer br.Close()

	if _, err = br.Exec(); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
			// Either the chat already exists or the other user does not exist
			if pgErr.ConstraintName == "users_uq" {
				return nil, status.Error(codes.AlreadyExists, "chat already exists")
			} else if pgErr.ConstraintName == constraintName {
				return nil, status.Error(codes.NotFound, "user does not exist")
			}
		}

		cs.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Error(codes.Internal, "could not create chat")
	}

	if _, err = br.Exec(); err != nil {
		cs.logger.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Error(codes.Internal, "could not create message")
	}

	// Close the batch to avoid memory leaks
	if err = br.Close(); err != nil {
		cs.logger.Errorf("Error in br.Close: %v", err)
		return nil, err
	}
	// Now we can commit the transaction
	if err = cs.db.CommitTx(ctx, tx); err != nil {
		cs.logger.Errorf("Error in cs.db.Commit: %v", err)
		return nil, err
	}

	// Send notification to the other user, so they know they have a new message
	notifCtx, notifSpan := cs.tracer.Start(ctx, "SendNotification")
	defer notifSpan.End()
	notifCtx = metadata.NewOutgoingContext(notifCtx, metadata.Pairs(string(keys.SubjectKey), req.GetUsername()))
	_, err = cs.notificationClient.SendNotification(notifCtx, &notificationv1.SendNotificationRequest{
		Recipient: req.GetUsername(),
		NotificationType: "message",
	})
	if err != nil {
		cs.logger.Errorf("Error in cs.notificationClient.SendNotification: %v", err)
		// Non-critical error, we don't return here
	}

	return &chatv1.CreateChatResponse{
		ChatId: chatId.String(),
		Message: &chatv1.ChatMessageResponse{
			Username:  username,
			Message:   req.GetMessage(),
			CreatedAt: createdAt.Format(time.RFC3339),
		},
	}, nil
}

// GetChat implements serveralpha.ChatServiceServer.
func (cs *chatService) GetChat(ctx context.Context, req *chatv1.GetChatRequest) (*chatv1.GetChatResponse, error) {
	conn, err := cs.db.Acquire(ctx)
	if err != nil {
		cs.logger.Errorf("Error in cs.db.Pool.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	offset, err := strconv.Atoi(req.GetPagination().GetPageToken())
	if err != nil {
		offset = 0
	}

	// Check if user1 or user2 equals the username, otherwise return not found
	selectCtx, selectSpan := cs.tracer.Start(ctx, "GetChat")
	dataQuery, dataArgs, _ := psql.Select().
		Columns("m.content", "m.created_at", "m.sender_name").
		From("messages m").
		Join("chats c ON m.chat_id = c.chat_id").
		Where("c.chat_id = ? AND (c.user1_name = ? OR c.user2_name = ?)", req.ChatId, username, username).
		OrderBy("m.created_at DESC").
		Offset(uint64(offset)).
		Limit(uint64(req.Pagination.GetPageSize())).
		ToSql()

	countQuery, countArgs, _ := psql.Select("COUNT(*)").
		From("messages m").
		Join("chats c ON m.chat_id = c.chat_id").
		Where("c.chat_id = ? AND (c.user1_name = ? OR c.user2_name = ?)", req.ChatId, username, username).
		ToSql()

	batch := pgx.Batch{}
	batch.Queue(dataQuery, dataArgs...)
	batch.Queue(countQuery, countArgs...)

	cs.logger.Info("Trying to get chat...")
	results := conn.SendBatch(selectCtx, &batch)
	defer results.Close()

	dataRows, err := results.Query()
	if err != nil {
		selectSpan.End()
		cs.logger.Errorf("Error in conn.SendBatch: %v", err)
		return nil, status.Error(codes.Internal, "could not get chat")
	}
	selectSpan.End()

	_, scanSpan := cs.tracer.Start(ctx, "ScanChatRows")
	var messages []*chatv1.ChatMessageResponse
	for dataRows.Next() {
		var message chatv1.ChatMessageResponse
		var createdAt pgtype.Timestamptz

		if err := dataRows.Scan(&message.Message, &createdAt, &message.Username); err != nil {
			scanSpan.End()
			cs.logger.Errorf("Error in dataRows.Scan: %v", err)
			return nil, status.Error(codes.Internal, "could not get chat")
		}

		if createdAt.Valid {
			message.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		messages = append(messages, &message)
	}

	pagination := &commonv1.PaginationResponse{
		NextPageToken: req.Pagination.GetPageToken(),
	}
	if err := results.QueryRow().Scan(&pagination.TotalSize); err != nil {
		scanSpan.End()
		cs.logger.Errorf("Error in results.QueryRow().Scan: %v", err)
		return nil, status.Error(codes.Internal, "could not get chat")
	}
	scanSpan.End()

	if pagination.GetTotalSize() == 0 {
		return nil, status.Error(codes.NotFound, "chat not found")
	}

	return &chatv1.GetChatResponse{
		Messages:   messages,
		Pagination: pagination,
	}, nil
}

// ListChats implements serveralpha.ChatServiceServer.
func (cs *chatService) ListChats(ctx context.Context, req *chatv1.ListChatsRequest) (*chatv1.ListChatsResponse, error) {
	conn, err := cs.db.Acquire(ctx)
	if err != nil {
		cs.logger.Errorf("Error in cs.db.Pool.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// We always need to return the chat_id and the user information about the other user. To determine
	// if the current user is user1 or user2, we use a common table expression (CTE) to get the other username.
	selectCtx, selectSpan := cs.tracer.Start(ctx, "GetChatData")
	query, args, _ := psql.Select("uc.chat_id").
		Column("uc.other_username").
		From("user_chats uc").
		Prefix(`
			WITH user_chats AS (
				SELECT chat_id, CASE
					WHEN user1_name = ? THEN user2_name
					ELSE user1_name
				END AS other_username
				FROM chats
				WHERE user1_name = ? OR user2_name = ?
			)
		`, username, username, username).
		ToSql()

	cs.logger.Info("Trying to list chats...")
	rows, err := conn.Query(selectCtx, query, args...)
	if err != nil {
		selectSpan.End()
		cs.logger.Errorf("Error in conn.Query: %v", err)
		return nil, status.Error(codes.Internal, "could not list chats")
	}
	selectSpan.End()

	_, scanSpan := cs.tracer.Start(ctx, "ScanChatRows")
	var chats []*chatv1.Chat
	var usernames []string
	for rows.Next() {
		var chat = chatv1.Chat{
			User: &userv1.User{},
		}

		if err := rows.Scan(&chat.Id, &chat.User.Username); err != nil {
			scanSpan.End()
			cs.logger.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Error(codes.Internal, "could not list chats")
		}

		chats = append(chats, &chat)
		usernames = append(usernames, chat.User.Username)
	}
	scanSpan.End()

	// Early return to avoid unnecessary calls to user service
	if len(chats) == 0 {
		return &chatv1.ListChatsResponse{
			Chats: chats,
		}, nil
	}

	// Get user information for each chat
	upstreamCtx, upstreamSpan := cs.tracer.Start(ctx, "UpstreamListUsers")
	resp, err := cs.userClient.ListUsers(upstreamCtx, &userv1.ListUsersRequest{Usernames: usernames})
	if err != nil {
		upstreamSpan.End()
		cs.logger.Errorf("Error in cs.userClient.ListUsers: %v", err)
		return nil, status.Error(codes.Internal, "could not list chats")
	}

	for _, chat := range chats {
		for _, user := range resp.Users {
			if chat.User.Username == user.Username {
				chat.User.Nickname = user.Nickname
				chat.User.Picture = user.Picture
				break
			}
		}
	}
	upstreamSpan.End()

	return &chatv1.ListChatsResponse{
		Chats: chats,
	}, nil
}

// PrepareChatStream implements serveralpha.ChatServiceServer.
func (cs *chatService) PrepareChatStream(ctx context.Context, req *chatv1.PrepareChatStreamRequest) (*chatv1.PrepareChatStreamResponse, error) {
	conn, err := cs.db.Acquire(ctx)
	if err != nil {
		cs.logger.Errorf("Error in cs.db.Pool.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Check if the chat exists and the user is part of it
	selectCtx, selectSpan := cs.tracer.Start(ctx, "SelectChatInfo")
	query, args, _ := psql.Select("user1_name, user2_name").
		From("chats").
		Where("chat_id = ?", req.GetChatId()).
		ToSql()

	cs.logger.Info("Trying to prepare chat stream...")
	username1, username2 := "", ""
	if err := conn.QueryRow(selectCtx, query, args...).Scan(&username1, &username2); err != nil {
		selectSpan.End()
		cs.logger.Errorf("Error in conn.QueryRow: %v", err)
		return nil, status.Error(codes.NotFound, "chat not found")
	}
	selectSpan.End()

	// Check if user1 or user2 equals the username, otherwise return not found
	if username1 != username && username2 != username {
		cs.logger.Warnf("User %s is not part of chat %s", username, req.GetChatId())
		return nil, status.Error(codes.NotFound, "chat not found")
	}

	// Create a new chat stream if it doesn't exist
	lockCtx, lockSpan := cs.tracer.Start(ctx, "LockChatStream")
	defer lockSpan.End() // since we'll unlock the mutex at the end of the function, we can defer the end of the span
	cs.mu.Lock()
	defer cs.mu.Unlock()
	_, prepareSpan := cs.tracer.Start(lockCtx, "PrepareChatStream")
	if _, ok := cs.streams[req.GetChatId()]; ok {
		// Chat was already prepared, return early
		prepareSpan.End()
		return &chatv1.PrepareChatStreamResponse{}, nil
	}

	// Create chat stream if it doesn't exist with two connections
	cs.streams[req.GetChatId()] = &chatStream{
		connections: []*openConnection{
			{username: username1, active: false, streams: nil},
			{username: username2, active: false, streams: nil},
		},
	}

	return &chatv1.PrepareChatStreamResponse{}, nil
}

// ChatMessage implements serveralpha.ChatServiceServer.
func (cs *chatService) ChatMessage(stream chatv1.ChatService_ChatMessageServer) error {
	ctx := stream.Context()
	cs.logger.Info("Received request for chat stream")

	// Get the metadata from the context
	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]
	chatId := metadata.ValueFromIncomingContext(ctx, string(keys.ChatIDKey))[0]

	// Check if the chat stream is prepared
	_, lockSpan := cs.tracer.Start(ctx, "LockChatStream")
	cs.mu.Lock()
	chatStream, ok := cs.streams[chatId]
	cs.mu.Unlock()
	lockSpan.End()
	if !ok {
		cs.logger.Error("Chat stream not prepared")
		return status.Error(codes.FailedPrecondition, "chat stream not prepared")
	}

	// Find the connection in the stream
	_, setupSpan := cs.tracer.Start(ctx, "SetupChatStream")
	var conn *openConnection
	for _, c := range chatStream.connections {
		if c.username == username {
			conn = c
			break
		}
	}

	if conn == nil {
		setupSpan.AddEvent("User not found in chat, prepare chat stream first")
		setupSpan.End()
		cs.logger.Error("User not found in chat, prepare chat stream first")
		return status.Error(codes.FailedPrecondition, "user not found in chat")
	}

	// Set the connection and mark it as active
	conn.streams = append(conn.streams, stream)
	conn.active = true
	cs.logger.Infof("Chat stream enabled for user %s", username)

	// Run handler routine concurrently
	routineCtx, cancel := context.WithCancel(ctx)
	go cs.handleMessages(routineCtx, cancel, chatId, conn, stream)

	// Wait for the stream to close
	<-routineCtx.Done()
	cs.logger.Info("Chat stream closed, deleting connection from internal state")

	// Delete current connection from the internal client state. If there are no more connections
	// for the chat, delete the chat from the internal state as well
	_, cleanupSpan := cs.tracer.Start(ctx, "CleanupChatStream")
	cs.mu.Lock()
	for i, currentConn := range cs.streams[chatId].connections {
		// Check for the correct connection in the state
		if currentConn == conn {
			for j, currentStream := range conn.streams {
				// We also need to check for the correct stream in the pool of streams for the connection
				if currentStream == stream {
					cleanupSpan.AddEvent(fmt.Sprintf("Deleting connection %d from chat %s", i, chatId))
					cs.logger.Infof("Deleting connection %d from chat %s", i, chatId)
					conn.streams = removeSingleStream(conn.streams, j)
					break
				}
			}

			if len(conn.streams) == 0 {
				cleanupSpan.AddEvent(fmt.Sprintf("No more streams for user %s in chat %s, setting to inactive", username, chatId))
				cs.logger.Infof("No more streams for user %s in chat %s, setting to inactive", username, chatId)
				conn.active = false
			}
		}
	}

	allInactive := true
	// Check if all connections for the chat are inactive. If so, delete the chat
	for _, c := range cs.streams[chatId].connections {
		if c.active {
			allInactive = false
			break
		}
	}
	if allInactive {
		cleanupSpan.AddEvent(fmt.Sprintf("All connections for chat %s are inactive, deleting chat", chatId))
		cs.logger.Infof("All connections for chat %s are inactive, deleting chat", chatId)
		delete(cs.streams, chatId)
	}

	cs.mu.Unlock()
	cs.logger.Info("Connection state updated")
	cleanupSpan.End()

	return nil
}

func (cs *chatService) handleMessages(ctx context.Context, cancel context.CancelFunc, chatId string, conn *openConnection, stream chatv1.ChatService_ChatMessageServer) {
	handleCtx, handleSpan := cs.tracer.Start(ctx, "HandleMessages")
	defer handleSpan.End()

	defer cancel()
	cs.logger.Infof("Starting message handler for user %s", conn.username)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Receive message from the client
			message, err := stream.Recv()

			chatCtx, chatSpan := cs.tracer.Start(handleCtx, "ReceiveMessage")
			if err != nil {
				chatSpan.AddEvent("Failed to receive message from client")
				chatSpan.End()
				cs.logger.Errorf("Failed to receive message from client: %v", err)
				return
			}

			// Check if the message is valid
			if message.GetUsername() != conn.username {
				chatSpan.AddEvent("Invalid message: username does not match")
				chatSpan.End()
				cs.logger.Error("Invalid message: username does not match")
				return
			}

			// Insert the message into the database
			dbConn, err := cs.db.Acquire(chatCtx)
			if err != nil {
				chatSpan.AddEvent("Failed to start connection")
				chatSpan.End()
				cs.logger.Errorf("Failed to start connection: %v", err)
				return
			}

			response := &chatv1.ChatMessageResponse{
				Username:  message.GetUsername(),
				Message:   message.GetMessage(),
				CreatedAt: time.Now().Format(time.RFC3339),
			}

			if err := cs.insertMessage(chatCtx, dbConn, chatId, response.GetUsername(), response.GetMessage(), response.GetCreatedAt()); err != nil {
				chatSpan.AddEvent("Failed to insert message")
				chatSpan.End()
				cs.logger.Errorf("Failed to insert message: %v", err)
				return
			}

			// Send the message to all open connections from the chat. This
			// also includes the sender, so the client knows that the message
			// was sent successfully. If the other client is not connected, send
			// a notification instead.
			sendCtx, sendSpan := cs.tracer.Start(chatCtx, "SendMessage")
			cs.mu.RLock()
			error := false
			for _, c := range cs.streams[chatId].connections {
				if c.active {
					for _, stream := range c.streams {
						if err := stream.Send(response); err != nil {
							sendSpan.AddEvent("Failed to send message to client")
							cs.logger.Errorf("Failed to send message to client: %v", err)
							error = true
							break
						}
						sendSpan.AddEvent(fmt.Sprintf("Sent message to %s", c.username))
					}
				// We run into this case if the connection is not active, but the user is still part of the chat
				// In this case we can't send a real-time message, but we can send a notification so the user knows
				// that they have a new message.
				} else if !c.active && c.username != conn.username {
					// Send notification to the other user if they are not connected
					notifCtx, notifSpan := cs.tracer.Start(sendCtx, "SendNotification")
					notifCtx = metadata.NewOutgoingContext(notifCtx, metadata.Pairs(string(keys.SubjectKey), c.username))
					_, err := cs.notificationClient.SendNotification(notifCtx, &notificationv1.SendNotificationRequest{
						Recipient: c.username,
						NotificationType: "message",
					})
					if err != nil {
						notifSpan.AddEvent("Failed to send notification")
						cs.logger.Errorf("Failed to send notification: %v", err)
						// This is not a critical error, so we don't return here
					}
					notifSpan.End()
				}
			}
			cs.mu.RUnlock()
			sendSpan.End()
			chatSpan.End()

			if error {
				return
			}
		}
	}
}

func (cs *chatService) insertMessage(ctx context.Context, conn *pgxpool.Conn, chatId, username, message, createdAt string) error {
	messageId := uuid.New()
	defer conn.Release()

	insertCtx, insertSpan := cs.tracer.Start(ctx, "InsertMessage")
	defer insertSpan.End()
	query, args, _ := psql.Insert("messages").
		Columns("chat_id", "message_id", "sender_name", "content", "created_at").
		Values(chatId, messageId, username, message, createdAt).
		ToSql()

	_, err := conn.Exec(insertCtx, query, args...)
	return err
}

func removeSingleStream(slice []chatv1.ChatService_ChatMessageServer, i int) []chatv1.ChatService_ChatMessageServer {
	slice[i] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}
