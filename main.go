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
	Name string `json:"name,omitempty"`
}

type Room struct {
	mu     sync.Mutex
	peers  [2]*websocket.Conn
	names  [2]string
	locked bool
}

var (
	rooms           = map[string]*Room{}
	roomsMutex      sync.RWMutex
	wsUpgrader      = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
)

func genID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, 6)
	for i := range bytes {
		bytes[i] = chars[rand.Intn(len(chars))]
	}
	return string(bytes)
}

func sendMsg(conn *websocket.Conn, msg Msg) {
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	roomID := genID()
	roomsMutex.Lock()
	rooms[roomID] = &Room{}
	roomsMutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": roomID})
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	roomsMutex.RLock()
	room, roomExists := rooms[roomID]
	roomsMutex.RUnlock()
	if !roomExists {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(4096)

	room.mu.Lock()
	peerSlot := -1
	if !room.locked {
		for slotIndex, existingConn := range room.peers {
			if existingConn == nil {
				peerSlot = slotIndex
				room.peers[slotIndex] = conn
				room.names[slotIndex] = genName(room.names[1-slotIndex])
				break
			}
		}
		if peerSlot == 1 {
			room.locked = true
		}
	}
	myName := room.names[peerSlot]
	var waitingPeerConn *websocket.Conn
	var waitingPeerName string
	if peerSlot == 1 {
		waitingPeerConn = room.peers[0]
		waitingPeerName = room.names[0]
	}
	room.mu.Unlock()

	if peerSlot < 0 {
		conn.Close()
		return
	}

	sendMsg(conn, Msg{Type: "welcome", Name: myName})

	if waitingPeerConn != nil {
		sendMsg(waitingPeerConn, Msg{Type: "peer_info", Text: "peer connected", Name: myName})
		sendMsg(conn, Msg{Type: "peer_info", Text: "peer connected", Name: waitingPeerName})
	}

	defer func() {
		conn.Close()
		room.mu.Lock()
		room.peers[peerSlot] = nil
		otherPeerConn := room.peers[1-peerSlot]
		roomIsEmpty := otherPeerConn == nil
		room.mu.Unlock()
		if otherPeerConn != nil {
			sendMsg(otherPeerConn, Msg{Type: "system", Text: "peer disconnected"})
		}
		if roomIsEmpty {
			roomsMutex.Lock()
			delete(rooms, roomID)
			roomsMutex.Unlock()
			log.Printf("room %s deleted", roomID)
		}
	}()

	for {
		_, messageData, err := conn.ReadMessage()
		if err != nil {
			break
		}
		room.mu.Lock()
		otherPeerConn := room.peers[1-peerSlot]
		room.mu.Unlock()
		if otherPeerConn != nil {
			otherPeerConn.WriteMessage(websocket.TextMessage, messageData)
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

	log.Println("listening on :8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
