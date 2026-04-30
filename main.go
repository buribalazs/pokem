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

type Peer struct {
	conn *websocket.Conn
	name string
}

type Room struct {
	mutex  sync.Mutex
	peers  [2]Peer
	locked bool
}

var (
	rooms      = map[string]*Room{}
	roomsMutex sync.RWMutex
	wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
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

	room.mutex.Lock()
	peerSlot := -1
	if !room.locked {
		for slotIndex, existingPeer := range room.peers {
			if existingPeer.conn == nil {
				peerSlot = slotIndex
				room.peers[slotIndex] = Peer{
					conn: conn,
					name: genName(room.peers[1-slotIndex].name),
				}
				break
			}
		}
		if peerSlot == 1 {
			room.locked = true
		}
	}
	var myPeer Peer
	var waitingPeer Peer
	if peerSlot >= 0 {
		myPeer = room.peers[peerSlot]
		if peerSlot == 1 {
			waitingPeer = room.peers[0]
		}
	}
	room.mutex.Unlock()

	if peerSlot < 0 {
		conn.Close()
		return
	}

	sendMsg(conn, Msg{Type: "welcome", Name: myPeer.name})

	if waitingPeer.conn != nil {
		sendMsg(waitingPeer.conn, Msg{Type: "peer_info", Text: "peer connected", Name: myPeer.name})
		sendMsg(conn, Msg{Type: "peer_info", Text: "peer connected", Name: waitingPeer.name})
	}

	defer func() {
		conn.Close()
		room.mutex.Lock()
		room.peers[peerSlot] = Peer{}
		otherPeer := room.peers[1-peerSlot]
		roomIsEmpty := otherPeer.conn == nil
		room.mutex.Unlock()
		if otherPeer.conn != nil {
			sendMsg(otherPeer.conn, Msg{Type: "system", Text: "peer disconnected"})
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
		room.mutex.Lock()
		otherPeer := room.peers[1-peerSlot]
		room.mutex.Unlock()
		if otherPeer.conn != nil {
			otherPeer.conn.WriteMessage(websocket.TextMessage, messageData)
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
