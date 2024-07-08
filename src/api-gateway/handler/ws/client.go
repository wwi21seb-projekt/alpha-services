package ws

import (
	"sync"
	"time"

	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	chatv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/chat/v1"

	"github.com/gorilla/websocket"
	"github.com/microcosm-cc/bluemonday"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
	"go.uber.org/zap"
)

type Client struct {
	// Central logger instance
	logger *zap.SugaredLogger
	// Central hub, which manages all clients
	hub *Hub
	// Username of the client
	username string
	// Websocket Connection to Client
	conn *websocket.Conn
	// gRPC Stream to Chat Service, which handles chat messages in both directions
	stream chatv1.ChatService_ChatMessageClient
	// Channel to send messages to client via websocket
	send chan []byte
	// Channel to signal client disconnect, used to close goroutines.
	Disconnect chan bool
	// This ensures that the disconnect channel is only closed once
	once sync.Once
	// Sanitizer for HTML content in messages
	policy *bluemonday.Policy
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// We only accept messages with 256 bytes of content, but we allow for
	// some overhead in our readLimit, so we can return a meaningful error
	// message to the client, instead of just closing the connection.
	readLimit = 4096
	// Maximum message size that we handle in the chat service
	maxMessageSize = 256
)

func NewClient(logger *zap.SugaredLogger, hub *Hub, conn *websocket.Conn, stream chatv1.ChatService_ChatMessageClient, username string) *Client {
	return &Client{
		logger:     logger,
		hub:        hub,
		conn:       conn,
		stream:     stream,
		username:   username,
		send:       make(chan []byte, 256),
		Disconnect: make(chan bool),
		once:       sync.Once{},
		policy:     bluemonday.UGCPolicy(),
	}
}

// readPump pumps messages from the websocket connection to the ChatService via gRPC.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) ReadPump(wg *sync.WaitGroup) {
	defer func() {
		c.triggerDisconnect()
		wg.Done()
		c.logger.Infof("ReadPump: Stopping read pump for client %s", c.username)
	}()
	c.logger.Infof("ReadPump: Starting read pump for client %s", c.username)
	c.conn.SetReadLimit(readLimit)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		c.logger.Infof("Received chat mesage from client: %s", c.username)
		if err != nil {
			c.logger.Errorf("Failed to read message from client: %v", err)
			return
		}

		// Unmarshal message and sanitize content
		var unmarshalledMessage schema.Message
		if err := unmarshalledMessage.UnmarshalJSON(message); err != nil {
			c.logger.Errorf("Failed to unmarshal message: %v", err)
			errorMessage := dto.ErrorDTO{Error: goerrors.BadRequest}
			errorMessageBytes, err := errorMessage.MarshalJSON()
			if err != nil {
				c.logger.Errorf("Failed to marshal error message: %v", err)
				return
			}
			c.send <- errorMessageBytes
			return
		}

		// Sanitize the message content
		unmarshalledMessage.Content = c.policy.Sanitize(unmarshalledMessage.Content)

		// Check if the message exceeds the maximum message size or is empty
		if len(unmarshalledMessage.Content) == 0 || len(unmarshalledMessage.Content) > maxMessageSize {
			c.logger.Errorf("Message exceeds maximum message size or is empty")
			errorMessage := dto.ErrorDTO{Error: goerrors.BadRequest}
			errorMessageBytes, err := errorMessage.MarshalJSON()
			if err != nil {
				c.logger.Errorf("Failed to marshal error message: %v", err)
				return
			}
			c.send <- errorMessageBytes
			continue
		}

		// Send it to the chat service via the open gRPC stream.
		if err := c.sendMessageToChatService(unmarshalledMessage); err != nil {
			c.logger.Errorf("Failed to send message to chat service: %v", err)
			return
		}
	}
}

// sendMessageToChatService sends a message from the client to the chat service via gRPC.
func (c *Client) sendMessageToChatService(message schema.Message) error {
	grpcMessage := &chatv1.ChatMessageRequest{
		Username: c.username,
		Message:  message.Content,
	}

	// Send the message to the chat service via gRPC
	err := c.stream.Send(grpcMessage)
	if err != nil {
		c.logger.Errorf("Failed to send message to chat service: %v", err)
		return err
	}
	return nil
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) WritePump(wg *sync.WaitGroup) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.triggerDisconnect()
		wg.Done()
		c.logger.Infof("WritePump: Stopping write pump for client %s", c.username)
	}()
	c.logger.Infof("WritePump: Starting write pump for client %s", c.username)
	for {
		select {
		// We receive two types of messages on the send channel: actual messages to be sent to the client
		// and errors that we want to send to the client. We handle both cases here.
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// Write occasional ping messages to the client to keep the connection alive
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) GrpcReceivePump(wg *sync.WaitGroup) {
	defer func() {
		c.triggerDisconnect()
		wg.Done()
		c.logger.Infof("GrpcReceivePump: Stopping grpc receive pump for client %s", c.username)
	}()
	c.logger.Infof("GrpcReceivePump: Starting grpc receive pump for client %s", c.username)

	for {
		// Enter an infinite loop to receive messages from the chat service via gRPC
		msg, err := c.stream.Recv()
		if err != nil {
			c.logger.Errorf("Failed to receive message from stream: %v", err)
			break
		}

		wsMessage := schema.Message{
			Username:     msg.Username,
			Content:      msg.Message,
			CreationDate: msg.CreatedAt,
		}
		wsMessageBytes, err := wsMessage.MarshalJSON()
		if err != nil {
			c.logger.Errorf("Failed to marshal message to json: %v", err)
			break
		}

		// Now that we have the message, we write it into the send channel to let our
		// write pump handle the message and send it to the client. We wrap this in a
		// select statement to ensure that we do not block if the send channel is full.
		select {
		case c.send <- wsMessageBytes:
		default:
			return
		}
	}
}

func (c *Client) triggerDisconnect() {
	c.once.Do(func() {
		close(c.Disconnect)
		c.logger.Infof("Disconnect triggered for client %s", c.username)
	})
}

func (c *Client) Close() {
	close(c.send)
	c.conn.Close()
	c.stream.CloseSend()
}
