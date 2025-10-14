// server.go
package main

import (
	"encoding/json"
	"log"
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
	PublicKey string `json:"publicKey,omitempty"`
	Payload   string `json:"payload,omitempty"`
}

func broadcast(exclude *websocket.Conn, msg WSMessage) {
	data, _ := json.Marshal(msg)
	mu.Lock()
	defer mu.Unlock()
	for conn := range clients {
		if conn != exclude {
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func sendToUser(username string, msg WSMessage) {
	data, _ := json.Marshal(msg)
	mu.Lock()
	defer mu.Unlock()
	for conn, name := range clients {
		if name == username {
			conn.WriteMessage(websocket.TextMessage, data)
			break
		}
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed:", err)
		return
	}

	defer func() {
		mu.Lock()
		if name, ok := clients[conn]; ok {
			delete(clients, conn)
			delete(publicKeys, name)
			mu.Unlock()
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
	http.HandleFunc("/ws", handleWS)
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
