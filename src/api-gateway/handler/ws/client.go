package ws

import (
	"bytes"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
)

type Client struct {
	// Central hub, which manages all clients
	hub *Hub
	// Username of the client
	username string
	// Websocket Connection to Client
	conn *websocket.Conn
	// gRPC Stream to Chat Service, which handles chat messages in both directions
	stream pb.ChatService_ChatStreamClient
	// Channel to send messages to client via websocket
	send chan []byte
	// Channel to signal client disconnect, used to close goroutines.
	Disconnect chan bool
	// This ensures that the disconnect channel is only closed once
	once sync.Once
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
)

func NewClient(hub *Hub, conn *websocket.Conn, stream pb.ChatService_ChatStreamClient, username string) *Client {
	return &Client{
		hub:        hub,
		conn:       conn,
		stream:     stream,
		username:   username,
		send:       make(chan []byte, 256),
		Disconnect: make(chan bool),
		once:       sync.Once{},
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
		log.Infof("ReadPump: Stopping read pump for client %s", c.username)
	}()
	log.Infof("ReadPump: Starting read pump for client %s", c.username)
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		log.Infof("Received chat mesage from client: %s", c.username)
		if err != nil {
			log.Errorf("Failed to read message from client: %v", err)
			return
		}
		// Trim any leading or trailing whitespace from the message and
		// send it to the chat service via the open gRPC stream.
		message = bytes.TrimSpace(message)
		if err := c.sendMessageToChatService(message); err != nil {
			log.Errorf("Failed to send message to chat service: %v", err)
			return
		}
	}
}

// sendMessageToChatService sends a message from the client to the chat service via gRPC.
func (c *Client) sendMessageToChatService(message []byte) error {
	// Prepare a message to be sent via gRPC
	var unmarshalledMessage schema.Message
	if err := unmarshalledMessage.UnmarshalJSON(message); err != nil {
		log.Errorf("Failed to unmarshal message: %v", err)
		return err
	}

	grpcMessage := &pb.ChatMessage{
		Username:  c.username,
		Message:   unmarshalledMessage.Content,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Send the message to the chat service via gRPC
	err := c.stream.Send(grpcMessage)
	if err != nil {
		log.Errorf("Failed to send message to chat service: %v", err)
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
		log.Infof("WritePump: Stopping write pump for client %s", c.username)
	}()
	log.Infof("WritePump: Starting write pump for client %s", c.username)
	for {
		select {
		// We receive messages from the gRPC stream and write them to the websocket connection
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

			wsMesage := schema.Message{}
			if err := wsMesage.UnmarshalJSON(message); err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

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
		log.Infof("GrpcReceivePump: Stopping grpc receive pump for client %s", c.username)
	}()
	log.Infof("GrpcReceivePump: Starting grpc receive pump for client %s", c.username)

	for {
		// Enter an infinite loop to receive messages from the chat service via gRPC
		msg, err := c.stream.Recv()
		if err != nil {
			log.Errorf("Failed to receive message from stream: %v", err)
			break
		}

		wsMessage := schema.Message{
			Username:     msg.Username,
			Content:      msg.Message,
			CreationDate: msg.CreatedAt,
		}
		wsMessageBytes, err := wsMessage.MarshalJSON()
		if err != nil {
			log.Errorf("Failed to marshal message to json: %v", err)
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
		log.Infof("Disconnect triggered for client %s", c.username)
	})
}

func (c *Client) Close() {
	close(c.send)
	c.conn.Close()
	c.stream.CloseSend()
}
