package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var ctx = context.Background()

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var redisClient *redis.Client

type Message struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan Message)
var mutex = sync.Mutex{}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	mutex.Lock()
	clients[ws] = true
	mutex.Unlock()

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("ReadJSON error: %v", err)
			mutex.Lock()
			delete(clients, ws)
			mutex.Unlock()
			break
		}

		log.Printf("Received message: %+v", msg)

		if err := saveMessage(msg); err != nil {
			log.Printf("Could not save message to Redis: %v", err)
		}

		broadcast <- msg
	}
}

func saveMessage(msg Message) error {
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("could not marshal message: %v", err)
	}

	_, err = redisClient.RPush(ctx, fmt.Sprintf("chat:%s", msg.Username), jsonMsg).Result()
	return err
}

func handleMessages() {
	for {
		msg := <-broadcast
		log.Printf("Broadcasting message: %+v", msg)
		mutex.Lock()
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("WriteJSON error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
		mutex.Unlock()
	}
}

func main() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // No password set
		DB:       0,  // use default DB
	})

	http.HandleFunc("/ws", handleConnections)

	go handleMessages()

	port := 3000
	fmt.Printf("Server is running on port %d\n", port)
	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
