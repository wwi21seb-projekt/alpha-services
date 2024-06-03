package handler

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"
	"github.com/wwi21seb-projekt/alpha-shared/db"
	"github.com/wwi21seb-projekt/alpha-shared/keys"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type chatService struct {
	db         *db.DB
	userClient pbUser.UserServiceClient
	pb.UnimplementedChatServiceServer
}

func NewChatService(db *db.DB, userClient pbUser.UserServiceClient) pb.ChatServiceServer {
	return &chatService{
		db:         db,
		userClient: userClient,
	}
}

// CreateChat implements serveralpha.ChatServiceServer.
func (c *chatService) CreateChat(ctx context.Context, req *pb.CreateChatRequest) (*pb.CreateChatResponse, error) {
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer c.db.Rollback(ctx, tx)

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
	log.Info("Trying to create chat and initial message...")
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

		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Error(codes.Internal, "could not create chat")
	}

	if _, err = br.Exec(); err != nil {
		log.Errorf("Error in tx.Exec: %v", err)
		return nil, status.Error(codes.Internal, "could not create message")
	}

	// Close the batch to avoid memory leaks
	if err = br.Close(); err != nil {
		log.Errorf("Error in br.Close: %v", err)
		return nil, err
	}
	// Now we can commit the transaction
	if err := c.db.Commit(ctx, tx); err != nil {
		log.Errorf("Error in c.db.Commit: %v", err)
		return nil, err
	}

	return &pb.CreateChatResponse{
		ChatId: chatId.String(),
		Message: &pb.ChatMessage{
			Username:  username,
			Message:   req.GetMessage(),
			CreatedAt: createdAt.Format(time.RFC3339),
		},
	}, nil
}

// GetChat implements serveralpha.ChatServiceServer.
func (c *chatService) GetChat(ctx context.Context, req *pb.GetChatRequest) (*pb.GetChatResponse, error) {
	conn, err := c.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("Error in c.db.Pool.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// Check if user1 or user2 equals the username, otherwise return not found
	dataQuery, dataArgs, _ := psql.Select().
		Columns("m.content", "m.created_at", "m.sender_name").
		From("messages m").
		Join("chats c ON m.chat_id = c.chat_id").
		Where("c.chat_id = ? AND (c.user1_name = ? OR c.user2_name = ?)", req.ChatId, username, username).
		OrderBy("m.created_at").
		Offset(uint64(req.Pagination.Offset)).
		Limit(uint64(req.Pagination.Limit)).
		ToSql()

	countQuery, countArgs, _ := psql.Select("COUNT(*)").
		From("messages").
		Where("chat_id = ?", req.ChatId).
		ToSql()

	batch := pgx.Batch{}
	batch.Queue(dataQuery, dataArgs...)
	batch.Queue(countQuery, countArgs...)

	log.Info("Trying to get chat...")
	results := conn.SendBatch(ctx, &batch)
	defer results.Close()

	dataRows, err := results.Query()
	if err != nil {
		log.Errorf("Error in conn.SendBatch: %v", err)
		return nil, status.Error(codes.Internal, "could not get chat")
	}

	var messages []*pb.ChatMessage
	for dataRows.Next() {
		var message pb.ChatMessage
		var createdAt pgtype.Timestamptz

		if err := dataRows.Scan(&message.Message, &createdAt, &message.Username); err != nil {
			log.Errorf("Error in dataRows.Scan: %v", err)
			return nil, status.Error(codes.Internal, "could not get chat")
		}

		if createdAt.Valid {
			message.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		messages = append(messages, &message)
	}

	pagination := &pbCommon.Pagination{
		Offset: req.Pagination.Offset,
		Limit:  req.Pagination.Limit,
	}
	if err := results.QueryRow().Scan(&pagination.Records); err != nil {
		log.Errorf("Error in results.QueryRow().Scan: %v", err)
		return nil, status.Error(codes.Internal, "could not get chat")
	}

	if pagination.Records == 0 {
		return nil, status.Error(codes.NotFound, "chat not found")
	}

	return &pb.GetChatResponse{
		Messages:   messages,
		Pagination: pagination,
	}, nil
}

// ListChats implements serveralpha.ChatServiceServer.
func (c *chatService) ListChats(ctx context.Context, req *pbCommon.Empty) (*pb.ListChatsResponse, error) {
	conn, err := c.db.Pool.Acquire(ctx)
	if err != nil {
		log.Errorf("Error in c.db.Pool.Acquire: %v", err)
		return nil, err
	}
	defer conn.Release()

	username := metadata.ValueFromIncomingContext(ctx, string(keys.SubjectKey))[0]

	// We always need to return the chat_id and the user information about the other user. To determine
	// if the current user is user1 or user2, we use a common table expression (CTE) to get the other username.
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

	log.Info("Trying to list chats...")
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		log.Errorf("Error in conn.Query: %v", err)
		return nil, status.Error(codes.Internal, "could not list chats")
	}

	var chats []*pb.Chat
	var usernames []string
	for rows.Next() {
		var chat = pb.Chat{
			User: &pbUser.User{},
		}

		if err := rows.Scan(&chat.Id, &chat.User.Username); err != nil {
			log.Errorf("Error in rows.Scan: %v", err)
			return nil, status.Error(codes.Internal, "could not list chats")
		}

		chats = append(chats, &chat)
		usernames = append(usernames, chat.User.Username)
	}

	// Early return to avoid unnecessary calls to user service
	if len(chats) == 0 {
		return &pb.ListChatsResponse{
			Chats: chats,
		}, nil
	}

	// Get user information for each chat
	resp, err := c.userClient.ListUsers(ctx, &pbUser.ListUsersRequest{Usernames: usernames})
	if err != nil {
		log.Errorf("Error in c.userClient.ListUsers: %v", err)
		return nil, status.Error(codes.Internal, "could not list chats")
	}

	for _, chat := range chats {
		for _, user := range resp.Users {
			if chat.User.Username == user.Username {
				chat.User.Nickname = user.Nickname
				chat.User.ProfilePictureUrl = user.ProfilePictureUrl
				break
			}
		}
	}

	return &pb.ListChatsResponse{
		Chats: chats,
	}, nil
}

// PrepareChatStream implements serveralpha.ChatServiceServer.
func (c *chatService) PrepareChatStream(ctx context.Context, req *pb.PrepareChatStreamRequest) (*pbCommon.Empty, error) {
	panic("unimplemented")
}

// ChatStream implements serveralpha.ChatServiceServer.
func (c *chatService) ChatStream(pb.ChatService_ChatStreamServer) error {
	panic("unimplemented")
}