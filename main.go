package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Room struct {
	mu    sync.Mutex
	peers [2]*websocket.Conn
}

var (
	rooms    = map[string]*Room{}
	roomsMu  sync.RWMutex
	upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
)

func genID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func writeMsg(conn *websocket.Conn, t, text string) {
	data, _ := json.Marshal(Msg{Type: t, Text: text})
	conn.WriteMessage(websocket.TextMessage, data)
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	id := genID()
	roomsMu.Lock()
	rooms[id] = &Room{}
	roomsMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	roomsMu.RLock()
	room, ok := rooms[id]
	roomsMu.RUnlock()
	if !ok {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	room.mu.Lock()
	slot := -1
	for i, p := range room.peers {
		if p == nil {
			slot = i
			room.peers[i] = conn
			break
		}
	}
	var other0 *websocket.Conn
	if slot == 1 {
		other0 = room.peers[0]
	}
	room.mu.Unlock()

	if slot < 0 {
		conn.Close()
		return
	}

	if other0 != nil {
		writeMsg(other0, "system", "peer connected")
		writeMsg(conn, "system", "peer connected")
	}

	defer func() {
		conn.Close()
		room.mu.Lock()
		room.peers[slot] = nil
		other := room.peers[1-slot]
		room.mu.Unlock()
		if other != nil {
			writeMsg(other, "system", "peer disconnected")
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		room.mu.Lock()
		other := room.peers[1-slot]
		room.mu.Unlock()
		if other != nil {
			other.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/room", handleCreateRoom)
	mux.HandleFunc("/ws/{id}", handleWS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
