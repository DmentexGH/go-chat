// main.go
package main

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	mu       sync.RWMutex
	rooms    = make(map[string]*Room)
)

type Room struct {
	mu         sync.RWMutex
	publicKeys map[string]string
	clients    map[*websocket.Conn]string
}

type WSMessage struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	RoomID    string `json:"roomId,omitempty"`
	Payload   string `json:"payload,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`
}

func broadcast(room *Room, exclude *websocket.Conn, msg WSMessage) {
	// Handle nil room gracefully
	if room == nil {
		return
	}

	room.mu.RLock()
	// Create a snapshot of connections to avoid holding lock during writes
	connections := make([]*websocket.Conn, 0, len(room.clients))
	for conn := range room.clients {
		if conn != exclude {
			connections = append(connections, conn)
		}
	}
	room.mu.RUnlock()

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

func sendToUser(room *Room, username string, msg WSMessage) {
	// Handle nil room gracefully
	if room == nil {
		return
	}

	room.mu.RLock()
	var targetConn *websocket.Conn
	for conn, name := range room.clients {
		if name == username {
			targetConn = conn
			break
		}
	}
	room.mu.RUnlock()

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

	var room *Room
	var username string
	var roomID string

	defer func() {
		if room != nil && username != "" {
			room.mu.Lock()
			if _, ok := room.clients[conn]; ok {
				delete(room.clients, conn)
				delete(room.publicKeys, username)
				// Check if room is empty and delete it
				if len(room.clients) == 0 {
					mu.Lock()
					delete(rooms, roomID)
					mu.Unlock()
				}
				room.mu.Unlock()
				conn.Close()
				broadcast(room, conn, WSMessage{Type: "leave", From: username})
			} else {
				room.mu.Unlock()
			}
		}
	}()

	var join WSMessage
	if err := conn.ReadJSON(&join); err != nil || join.Type != "join" || join.From == "" || join.PublicKey == "" || join.RoomID == "" {
		return
	}

	username = join.From
	roomID = join.RoomID

	mu.Lock()
	// Get or create room
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{
			publicKeys: make(map[string]string),
			clients:    make(map[*websocket.Conn]string),
		}
		rooms[roomID] = room
	}
	mu.Unlock()

	room.mu.Lock()
	// Check if username is taken in this room
	if _, exists := room.publicKeys[username]; exists {
		room.mu.Unlock()
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"Username taken in this room"}`))
		return
	}

	room.clients[conn] = username
	room.publicKeys[username] = join.PublicKey
	room.mu.Unlock()

	// Announce join to room
	broadcast(room, conn, WSMessage{
		Type:      "join",
		From:      username,
		RoomID:    roomID,
		PublicKey: join.PublicKey,
	})

	// Send all existing public keys in room to new user
	room.mu.RLock()
	for name, pubKey := range room.publicKeys {
		if name != username {
			conn.WriteJSON(WSMessage{
				Type:      "pubkey",
				From:      name,
				RoomID:    roomID,
				PublicKey: pubKey,
			})
		}
	}
	room.mu.RUnlock()

	// Relay messages
	for {
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Type == "chat" && msg.Payload != "" {
			// Send to specified user in the same room
			if msg.To != "" {
				sendToUser(room, msg.To, WSMessage{
					Type:    "chat",
					From:    username,
					RoomID:  roomID,
					Payload: msg.Payload,
				})
			}
		} else if msg.Type == "clear" {
			broadcast(room, nil, WSMessage{Type: "clear", RoomID: roomID})
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
