package ws

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool
	// Register requests from the clients.
	Register chan *Client
	// Unregister requests from clients.
	Unregister chan *Client
	// Mutex to protect access to clients
	clientsMu sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		clientsMu:  sync.Mutex{},
	}
}

func (h *Hub) Run() {
	log.Info("Running chat hub...")

	for {
		select {
		case client := <-h.Register:
			log.Info("Registering client")
			h.clientsMu.Lock()
			h.clients[client] = true
			h.clientsMu.Unlock()
		case client := <-h.Unregister:
			log.Infof("Unregistering client %v", client.Username)

			h.clientsMu.Lock()
			if _, ok := h.clients[client]; ok {
				// Clean up open connections and channels
				close(client.Send)
				close(client.Disconnect)
				delete(h.clients, client)
				client.Conn.Close()
				client.Stream.CloseSend()
				log.Infof("Client %s unregistered", client.Username)
			}
			h.clientsMu.Unlock()
		}
	}
}

func (h *Hub) Close() {
	log.Info("Closing chat hub...")

	// Close the Register channel to prevent new clients from connecting
	// Keep the Unregister channel open for now to allow clients to disconnect
	close(h.Register)

	// Close all client connections
	log.Info("Closing all open client connections...")
	h.clientsMu.Lock()
	for client := range h.clients {
		h.Unregister <- client
	}
	close(h.Unregister)
	h.clientsMu.Unlock()
}
