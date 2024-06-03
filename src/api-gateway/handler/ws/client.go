package ws

import (
	"bytes"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/chat"
)

type Client struct {
	Hub        *Hub                            // Chat Hub
	Username   string                          // Username of the client
	Conn       *websocket.Conn                 // Websocket Connection to Client
	Stream     pb.ChatService_ChatStreamClient // gRPC Stream to Chat Service
	Send       chan []byte                     // Channel to send messages to client
	Disconnect chan bool                       // Channel to signal client disconnect
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
	space   = []byte{' '}
)

func NewClient(hub *Hub, conn *websocket.Conn, stream pb.ChatService_ChatStreamClient, username string) *Client {
	return &Client{
		Hub:      hub,
		Conn:     conn,
		Stream:   stream,
		Username: username,
		Send:     make(chan []byte, 256),
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) ReadPump() {
	defer func() {
		c.Disconnect <- true
	}()
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.Conn.ReadMessage()
		log.Infof("Received chat mesage from client: %s", c.Username)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			return
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		if err := c.sendMessageToChatService(message); err != nil {
			log.Errorf("Failed to send message to chat service: %v", err)
			return
		}
	}
}

// sendMessageToChatService sends a message from the client to the chat service via gRPC.
func (c *Client) sendMessageToChatService(message []byte) error {
	// Prepare a message to be sent via gRPC
	grpcMessage := &pb.ChatMessage{
		Username:  c.Username,
		Message:   string(message),
		CreatedAt: time.Now().String(),
	}

	// Send the message to the chat service via gRPC
	err := c.Stream.Send(grpcMessage)
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
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Disconnect <- true
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}

			wsMesage := schema.Message{}
			if err := wsMesage.UnmarshalJSON(message); err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) GrpcReceivePump() {
	defer func() {
		c.Disconnect <- true
	}()

	for {
		msg, err := c.Stream.Recv()
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

		select {
		case c.Send <- wsMessageBytes:
		default:
			return
		}
	}
}
