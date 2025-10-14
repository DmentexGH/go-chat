// server.go
package main

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	upgrader   = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	mu         sync.Mutex
	publicKeys = make(map[string]string)
	clients    = make(map[*websocket.Conn]string)
)

type WSMessage struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Payload   string `json:"payload,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`
}

func broadcast(exclude *websocket.Conn, msg WSMessage) {
	mu.Lock()
	// Create a snapshot of connections to avoid holding lock during writes
	connections := make([]*websocket.Conn, 0, len(clients))
	for conn := range clients {
		if conn != exclude {
			connections = append(connections, conn)
		}
	}
	mu.Unlock()

	// Send messages without holding the lock
	for _, conn := range connections {
		// Use goroutine to avoid blocking on slow clients
		go func(c *websocket.Conn) {
			// Recover from any panics during message sending
			defer func() {
				if r := recover(); r != nil {
					// Silent recovery - continue with other connections
				}
			}()
			c.WriteJSON(msg)
		}(conn)
	}
}

func sendToUser(username string, msg WSMessage) {
	mu.Lock()
	var targetConn *websocket.Conn
	for conn, name := range clients {
		if name == username {
			targetConn = conn
			break
		}
	}
	mu.Unlock()

	if targetConn != nil {
		go func() {
			// Recover from any panics during message sending
			defer func() {
				if r := recover(); r != nil {
					// Silent recovery - continue with other operations
				}
			}()
			targetConn.WriteJSON(msg)
		}()
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	// Recover from any panics in WebSocket handling
	defer func() {
		if r := recover(); r != nil {
			// Silent recovery - continue serving other connections
		}
	}()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	defer func() {
		mu.Lock()
		if name, ok := clients[conn]; ok {
			delete(clients, conn)
			delete(publicKeys, name)
			mu.Unlock()
			conn.Close()
			broadcast(nil, WSMessage{Type: "leave", From: name})
		} else {
			mu.Unlock()
		}
	}()

	var join WSMessage
	if err := conn.ReadJSON(&join); err != nil || join.Type != "join" || join.From == "" || join.PublicKey == "" {
		return
	}

	mu.Lock()
	if _, exists := publicKeys[join.From]; exists {
		mu.Unlock()
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"Username taken"}`))
		return
	}

	clients[conn] = join.From
	publicKeys[join.From] = join.PublicKey
	mu.Unlock()
	// But keep join announcement
	broadcast(nil, WSMessage{
		Type:      "join",
		From:      join.From,
		PublicKey: join.PublicKey,
	})

	// Send all existing public keys to new user
	mu.Lock()
	for name, pubKey := range publicKeys {
		if name != join.From {
			conn.WriteJSON(WSMessage{
				Type:      "pubkey",
				From:      name,
				PublicKey: pubKey,
			})
		}
	}
	mu.Unlock()

	// Relay messages
	for {
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Type == "chat" && msg.Payload != "" {
			// Send to specified user
			if msg.To != "" {
				sendToUser(msg.To, WSMessage{
					Type:    "chat",
					From:    clients[conn],
					Payload: msg.Payload,
				})
			}
		} else if msg.Type == "clear" {
			broadcast(nil, WSMessage{Type: "clear"})
		}
	}
}

func main() {
	for {
		server := &http.Server{
			Addr:           ":8080",
			MaxHeaderBytes: 1 << 20, // 1MB
		}

		http.HandleFunc("/ws", handleWS)
		server.ListenAndServe()
		// If we reach here, the server crashed - restart automatically
	}
}
