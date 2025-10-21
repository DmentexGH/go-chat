package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to start a httptest server whose handler upgrades and registers the server-side conn into room.
// It returns the client-side websocket.Conn (for reading), the server-side websocket.Conn pointer (so tests can pass it as exclude),
// a channel which will receive WSMessage read by client, and a cleanup func.
func startRegisteredConn(t *testing.T, room *Room, username string) (clientConn *websocket.Conn, serverConn *websocket.Conn, recvCh chan WSMessage, cleanup func()) {
	recvCh = make(chan WSMessage, 10)
	registered := make(chan struct{})

	var srvConn *websocket.Conn
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		// register in room
		room.mu.Lock()
		room.clients[c] = username
		room.mu.Unlock()

		// store server conn for caller
		srvConn = c
		close(registered)

		// keep this handler alive until cleanup closes server
		// read loop to keep connection usable (we ignore incoming messages)
		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				// connection closed — exit handler
				return
			}
		}
	}))
	// dial client
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	cClient, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)

	// client reader goroutine
	go func() {
		for {
			var msg WSMessage
			if err := cClient.ReadJSON(&msg); err != nil {
				close(recvCh)
				return
			}
			recvCh <- msg
		}
	}()

	// wait for server-side to register
	select {
	case <-registered:
	case <-time.After(200 * time.Millisecond):
		// cleanup and fail if not registered
		server.Close()
		cClient.Close()
		t.Fatalf("server handler did not register connection in time")
	}

	cleanup = func() {
		// Close client side first to stop its reader
		_ = cClient.Close()
		// Close server: stops handler and closes server-side conns
		server.Close()
		// also close server-side conn if present
		if srvConn != nil {
			_ = srvConn.Close()
		}
		// drain recvCh
		time.Sleep(5 * time.Millisecond)
	}

	return cClient, srvConn, recvCh, cleanup
}

// TestRoomCreation ensures the global rooms map is manipulated correctly
func TestRoomCreation(t *testing.T) {
	mu.Lock()
	rooms = make(map[string]*Room)
	mu.Unlock()

	roomID := "test-room"

	mu.RLock()
	_, exists := rooms[roomID]
	mu.RUnlock()
	assert.False(t, exists)

	// Simulate creating a room (the server does this during join)
	mu.Lock()
	rooms[roomID] = &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}
	mu.Unlock()

	mu.RLock()
	room, exists := rooms[roomID]
	mu.RUnlock()

	assert.True(t, exists)
	require.NotNil(t, room)
	assert.NotNil(t, room.publicKeys)
	assert.NotNil(t, room.clients)
}

// TestBroadcastFunction: verifies broadcast sends to all clients and respects exclude
func TestBroadcastFunction(t *testing.T) {
	room := &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}

	// Start two registered connections
	_, serverConn1, ch1, cleanup1 := startRegisteredConn(t, room, "user1")
	defer cleanup1()

	_, _, ch2, cleanup2 := startRegisteredConn(t, room, "user2")
	defer cleanup2()

	// Give a bit of time for stability
	time.Sleep(10 * time.Millisecond)

	msg := WSMessage{
		Type:    "test",
		From:    "sender",
		Payload: "broadcast-all",
	}

	// Broadcast to all (exclude nil)
	broadcast(room, nil, msg)

	// both clients should receive
	select {
	case m := <-ch1:
		assert.Equal(t, msg.Type, m.Type)
		assert.Equal(t, msg.Payload, m.Payload)
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("client1 did not receive broadcast")
	}

	select {
	case m := <-ch2:
		assert.Equal(t, msg.Type, m.Type)
		assert.Equal(t, msg.Payload, m.Payload)
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("client2 did not receive broadcast")
	}

	// Broadcast excluding serverConn1: only client2 should receive
	msg2 := WSMessage{
		Type:    "test",
		From:    "sender",
		Payload: "exclude-one",
	}
	broadcast(room, serverConn1, msg2)

	// client1 should NOT receive (drain if any)
	select {
	case m := <-ch1:
		// if we do receive something here, ensure it's not the excluded payload
		if m.Payload == msg2.Payload {
			t.Fatalf("client1 unexpectedly received excluded broadcast")
		}
	default:
		// expected
	}

	// client2 must receive
	select {
	case m := <-ch2:
		assert.Equal(t, msg2.Payload, m.Payload)
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("client2 did not receive broadcast after exclusion")
	}

	// Test broadcasting to empty room (should not panic)
	emptyRoom := &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}
	broadcast(emptyRoom, nil, WSMessage{Type: "noop"})
}

// TestSendToUserFunction: verify sendToUser targets correct client and is a no-op for missing user
func TestSendToUserFunction(t *testing.T) {
	room := &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}

	_, _, recvCh, cleanup := startRegisteredConn(t, room, "target-user")
	defer cleanup()

	// send to existing user
	msg := WSMessage{
		Type:    "chat",
		From:    "sender",
		Payload: "private-hello",
	}
	sendToUser(room, "target-user", msg)

	select {
	case m := <-recvCh:
		assert.Equal(t, msg.Payload, m.Payload)
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("target-user did not receive message")
	}

	// send to non-existent user (should not panic or block)
	sendToUser(room, "no-one", msg)
}

// TestWSMessageStruct marshals/unmarshals as expected
func TestWSMessageStruct(t *testing.T) {
	joinMsg := WSMessage{
		Type:      "join",
		From:      "test-user",
		RoomID:    "test-room",
		PublicKey: "test-public-key",
	}

	data, err := json.Marshal(joinMsg)
	require.NoError(t, err)

	var decoded WSMessage
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, joinMsg.Type, decoded.Type)
	assert.Equal(t, joinMsg.From, decoded.From)
	assert.Equal(t, joinMsg.RoomID, decoded.RoomID)
	assert.Equal(t, joinMsg.PublicKey, decoded.PublicKey)

	chat := WSMessage{
		Type:    "chat",
		From:    "a",
		To:      "b",
		Payload: "hi",
	}
	data, err = json.Marshal(chat)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, chat.Type, decoded.Type)
	assert.Equal(t, chat.From, decoded.From)
	assert.Equal(t, chat.To, decoded.To)
	assert.Equal(t, chat.Payload, decoded.Payload)
}

// Integration tests against handleWS: join, duplicate username, invalid join, public-key exchange, room cleanup
func TestInvalidJoinMessage(t *testing.T) {
	mu.Lock()
	rooms = make(map[string]*Room)
	mu.Unlock()

	server := httptest.NewServer(http.HandlerFunc(handleWS))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")

	// Missing required fields: send {type: "join"} only
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	invalid := WSMessage{Type: "join"}
	require.NoError(t, conn.WriteJSON(invalid))

	// server should close connection or respond in a short time
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err)
	_ = conn.Close()

	// Empty username
	conn2, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn2.Close()

	invalid2 := WSMessage{
		Type:      "join",
		From:      "",
		RoomID:    "r",
		PublicKey: "k",
	}
	require.NoError(t, conn2.WriteJSON(invalid2))
	conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn2.ReadMessage()
	assert.Error(t, err)
	_ = conn2.Close()
}

func TestUsernameTakenAndPublicKeyExchange(t *testing.T) {
	mu.Lock()
	rooms = make(map[string]*Room)
	mu.Unlock()

	server := httptest.NewServer(http.HandlerFunc(handleWS))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	// First join
	conn1, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn1.Close()

	join1 := WSMessage{
		Type:      "join",
		From:      "alice",
		RoomID:    "room1",
		PublicKey: "pk-alice",
	}
	require.NoError(t, conn1.WriteJSON(join1))

	// second join with same username in same room -> expect error message
	conn2, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn2.Close()

	join2 := WSMessage{
		Type:      "join",
		From:      "alice", // same
		RoomID:    "room1",
		PublicKey: "pk-bad",
	}
	require.NoError(t, conn2.WriteJSON(join2))

	// read server response (should contain "Username taken")
	conn2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, message, err := conn2.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(message), "Username taken")

	// Now join a different user in same room; verify pubkey exchange occurs
	conn3, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn3.Close()

	join3 := WSMessage{
		Type:      "join",
		From:      "bob",
		RoomID:    "room1",
		PublicKey: "pk-bob",
	}
	require.NoError(t, conn3.WriteJSON(join3))

	// wait a bit for server to process the join
	time.Sleep(20 * time.Millisecond)

	// Verify server stored public keys
	mu.RLock()
	room, ok := rooms["room1"]
	mu.RUnlock()
	require.True(t, ok)

	room.mu.RLock()
	assert.Equal(t, "pk-alice", room.publicKeys["alice"])
	assert.Equal(t, "pk-bob", room.publicKeys["bob"])
	room.mu.RUnlock()
}

func TestRoomCleanup(t *testing.T) {
	mu.Lock()
	rooms = make(map[string]*Room)
	mu.Unlock()

	// create room and register a connection using helper server so we can ensure removal after close via handleWS cleanup behavior
	roomID := "cleanup-room"
	room := &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}
	mu.Lock()
	rooms[roomID] = room
	mu.Unlock()

	// start a handleWS server so we can join and then close connection to trigger cleanup
	server := httptest.NewServer(http.HandlerFunc(handleWS))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)

	join := WSMessage{
		Type:      "join",
		From:      "cleanup-user",
		RoomID:    roomID,
		PublicKey: "cleanup-key",
	}
	require.NoError(t, conn.WriteJSON(join))

	// wait for server to process
	time.Sleep(20 * time.Millisecond)

	// Now close connection to trigger deferred cleanup in handleWS
	require.NoError(t, conn.Close())

	// give server a moment to remove room if empty
	time.Sleep(30 * time.Millisecond)

	mu.RLock()
	_, exists := rooms[roomID]
	mu.RUnlock()

	// After the connection closed, room should be removed
	assert.False(t, exists)
}

// Test that handleWS doesn't panic on non-upgrade HTTP requests
func TestHandleWSNonUpgradeRequestsDoNotPanic(t *testing.T) {
	// simple GET should not panic
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	handleWS(w, req)

	// POST should also be handled gracefully
	req2 := httptest.NewRequest("POST", "/ws", nil)
	w2 := httptest.NewRecorder()
	handleWS(w2, req2)
}

func TestHandleWSInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(handleWS))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Send invalid JSON
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("{invalid-json")))

	// Expect server to close connection soon
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err)
}

func TestBroadcastWriteJSONFailure(t *testing.T) {
	room := &Room{
		publicKeys: make(map[string]string),
		clients:    make(map[*websocket.Conn]string),
	}

	client, _, _, cleanup := startRegisteredConn(t, room, "tester")
	defer cleanup()

	// Close client so WriteJSON fails
	_ = client.Close()

	// Short sleep so connection fully closes
	time.Sleep(10 * time.Millisecond)

	msg := WSMessage{Type: "chat", Payload: "should-fail"}
	// Should not panic or hang
	broadcast(room, nil, msg)

	// If we reach here without panic, success
}

func TestHandleWSUpgradeFailure(t *testing.T) {
	req := httptest.NewRequest("GET", "/ws", nil)
	// Deliberately omit WebSocket headers
	w := httptest.NewRecorder()

	// Should not panic or write unexpected output
	assert.NotPanics(t, func() {
		handleWS(w, req)
	})

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode) // default from Upgrade error
}
