// Package live provides the small WebSocket hub shared by the ceremony room
// and the radio room: identity-tracked clients, per-client write locking,
// broadcast with dead-client eviction, and ephemeral listener aliases.
package live

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Token generates an opaque random identifier for rooms and sessions.
func Token() string { return uuid.NewString() }

// Message is one frame sent to clients.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Client is one connected browser. Per-client write locking makes it safe
// for the hub and a join handler to write to the same connection.
type Client struct {
	conn    *websocket.Conn
	userID  string
	writeMu sync.Mutex
}

// SendJSON writes one message to this client, serialized against other writers.
func (c *Client) SendJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteJSON(v)
}

// UserID returns the identity this client joined with.
func (c *Client) UserID() string { return c.userID }

// CloseConnection closes the underlying connection.
func (c *Client) CloseConnection() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.Close()
}

// Room tracks the live WebSocket clients for one ceremony.
type Room struct {
	token   string
	mu      sync.Mutex
	clients map[*Client]struct{}
}

func newRoom(token string) *Room {
	return &Room{token: token, clients: map[*Client]struct{}{}}
}

// Join registers a connection for a user and returns its client handle.
func (r *Room) Join(conn *websocket.Conn, userID string) *Client {
	c := &Client{conn: conn, userID: userID}
	r.mu.Lock()
	r.clients[c] = struct{}{}
	r.mu.Unlock()
	return c
}

// Leave removes a client registered via Join.
func (r *Room) Leave(c *Client) {
	r.mu.Lock()
	delete(r.clients, c)
	r.mu.Unlock()
}

// Participants returns the distinct user IDs currently connected (in the room).
func (r *Room) Participants() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]struct{}{}
	var out []string
	for c := range r.clients {
		if _, dup := seen[c.userID]; dup {
			continue
		}
		seen[c.userID] = struct{}{}
		out = append(out, c.userID)
	}
	sort.Strings(out)
	return out
}

// Count returns how many connections are live.
func (r *Room) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients)
}

// Broadcast sends msg to every connected client, dropping dead ones.
func (r *Room) Broadcast(msgType string, payload any) {
	r.broadcast(msgType, payload, nil)
}

// BroadcastExcept is Broadcast for everyone but one client — used to notify
// the room about a join that the newcomer already received privately.
func (r *Room) BroadcastExcept(skip *Client, msgType string, payload any) {
	r.broadcast(msgType, payload, skip)
}

func (r *Room) broadcast(msgType string, payload any, skip *Client) {
	msg := Message{Type: msgType, Payload: payload}

	r.mu.Lock()
	clients := make([]*Client, 0, len(r.clients))
	for c := range r.clients {
		clients = append(clients, c)
	}
	r.mu.Unlock()

	var dead []*Client
	for _, c := range clients {
		if c == skip {
			continue
		}
		if err := c.SendJSON(msg); err != nil {
			dead = append(dead, c)
		}
	}
	if len(dead) == 0 {
		return
	}
	r.mu.Lock()
	for _, c := range dead {
		if _, ok := r.clients[c]; ok {
			delete(r.clients, c)
			r.mu.Unlock()
			log.Printf("live room %s: dropping dead client", r.token)
			c.CloseConnection()
			r.mu.Lock()
		}
	}
	r.mu.Unlock()
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
