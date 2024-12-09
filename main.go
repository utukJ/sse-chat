package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type WriteRequest struct {
	Msg string `json:"msg"`
}

type ReadClient struct {
	id      string
	msgChan chan string
	// expiry  time.Time
}

type Coordinator struct {
	msgs              []string
	newMsgChan        chan string
	readClients       map[string]ReadClient
	newReadClientChan chan ReadClient
}

type Server struct {
	coordinator *Coordinator
}

func (c *Coordinator) run() {
	logrus.Info("Starting coordinator...")
	for {
		select {
		case msg := <-c.newMsgChan:
			for _, client := range c.readClients {
				client.msgChan <- msg
			}
			c.msgs = append(c.msgs, msg)
		case client := <-c.newReadClientChan:
			c.readClients[client.id] = client
			logrus.Infof("New read client connected: %s", client.id)
			logrus.Infof("Current number of active clients: %d", len(c.readClients))
			for _, msg := range c.msgs {
				client.msgChan <- msg
			}
		}
	}
}

func (s *Server) landing(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "SSE Chat!")
}

func (s *Server) newMessage(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("Received new message request from %s", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")

	var wr WriteRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&wr)
	if err != nil {
		logrus.Warnf("Invalid request body from %s", r.RemoteAddr)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	logrus.Infof("Request body: %v", wr)
	s.coordinator.newMsgChan <- wr.Msg

	fmt.Fprintf(w, `{"status": "success"}`)
}

func (s *Server) readMessages(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("Received read messages request from %s", r.RemoteAddr)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id := uuid.New().String()

	msgChan := make(chan string)
	readClient := ReadClient{
		id:      id,
		msgChan: msgChan,
	}

	s.coordinator.newReadClientChan <- readClient
	logrus.Infof("New read client connected: %s", id)
	logrus.Infof("Current number of active clients: %d", len(s.coordinator.readClients))

	for {
		msg := <-msgChan
		fmt.Fprintf(w, ">> %s\n\n", msg)
		flusher, ok := w.(http.Flusher)
		if !ok {
			logrus.Errorf("Streaming unsupported for client %s", id)
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()
	}
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	c := Coordinator{
		msgs:              []string{"Hello SSE Chat!"},
		newMsgChan:        make(chan string),
		readClients:       make(map[string]ReadClient),
		newReadClientChan: make(chan ReadClient),
	}

	s := Server{
		coordinator: &c,
	}

	go c.run()

	r := mux.NewRouter()
	r.HandleFunc("/", s.landing)
	r.HandleFunc("/chats", s.newMessage).Methods("POST")
	r.HandleFunc("/chats", s.readMessages).Methods("GET")
	http.Handle("/", r)
	http.ListenAndServe(":8080", nil)
	http.ListenAndServe(":8080", nil)
}
