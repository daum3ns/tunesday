// Package ceremony implements the live provider-selection room: a small
// WebSocket hub that keeps every connected browser in sync during the reveal.
package ceremony

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message is one frame sent to clients.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Room tracks the live WebSocket clients for one ceremony.
type Room struct {
	token   string
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func newRoom(token string) *Room {
	return &Room{token: token, clients: map[*websocket.Conn]struct{}{}}
}

// Join registers a connection and sends it nothing (the handler sends initial state).
func (r *Room) Join(conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[conn] = struct{}{}
}

// Leave removes a connection.
func (r *Room) Leave(conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, conn)
}

// Count returns how many clients are connected.
func (r *Room) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients)
}

// Broadcast sends msg to every connected client, dropping dead ones.
func (r *Room) Broadcast(msgType string, payload any) {
	data, err := json.Marshal(Message{Type: msgType, Payload: payload})
	if err != nil {
		log.Printf("ceremony %s: marshal %s: %v", r.token, msgType, err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for conn := range r.clients {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			delete(r.clients, conn)
			_ = conn.Close()
		}
	}
}

// Manager owns the set of active ceremony rooms.
type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewManager creates an empty room manager.
func NewManager() *Manager {
	return &Manager{rooms: map[string]*Room{}}
}

// RoomFor returns the room for a ceremony token, creating it on first use.
func (m *Manager) RoomFor(token string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	room, ok := m.rooms[token]
	if !ok {
		room = newRoom(token)
		m.rooms[token] = room
	}
	return room
}

// Forget removes an empty room (ceremony completed).
func (m *Manager) Forget(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if room, ok := m.rooms[token]; ok && room.Count() == 0 {
		delete(m.rooms, token)
	}
}
