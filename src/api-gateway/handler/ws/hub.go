package ws

import (
	log "github.com/sirupsen/logrus"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool
	// Inbound messages from the clients.
	broadcast chan []byte
	// Register requests from the clients.
	Register chan *Client
	// Unregister requests from clients.
	Unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	log.Info("Running chat hub...")

	for {
		select {
		case client := <-h.Register:
			log.Info("Registering client")
			h.clients[client] = true
		case client := <-h.Unregister:
			log.Infof("Unregistering client %v", client.Username)

			if _, ok := h.clients[client]; ok {
				// Clean up open connections and channels
				close(client.Send)
				delete(h.clients, client)
				client.Conn.Close()
				client.Stream.CloseSend()
				log.Info("Client unregistered")
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) Close() {
	log.Info("Closing chat hub...")

	close(h.broadcast)
	close(h.Register)
	close(h.Unregister)

	// Close all client connections
	log.Info("Closing all open client connections...")
	for client := range h.clients {
		close(client.Send)
		delete(h.clients, client)
		client.Conn.Close()
		client.Stream.CloseSend()
	}
}
