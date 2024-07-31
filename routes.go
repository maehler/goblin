package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

//go:embed templates
var templates embed.FS

//go:embed all:static
var static embed.FS

type subscriber struct {
	messages chan *Message
	ip       string
}

type server struct {
	nexa            *Nexa
	mux             *http.ServeMux
	subscriberMutex sync.Mutex
	subscribers     map[subscriber]bool
}

func (s *server) nodesHandler(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.nexa.Nodes()
	if err != nil {
		w.Write([]byte(fmt.Sprintf("error: %+v", err.Error())))
		return
	}
	w.Write([]byte(fmt.Sprintf("%+v", nodes)))
}

func (s *server) roomsHandler(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.nexa.Rooms()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("error: %+v", err.Error())))
		return
	}
	t := template.Must(template.ParseFS(
		templates,
		"templates/layout.tmpl",
		"templates/rooms.tmpl",
	))
	t.Execute(w, rooms)
}

func (s *server) deviceHandler(w http.ResponseWriter, r *http.Request) {
	device, err := s.nexa.Node(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("error: %+v", err.Error())))
		return
	}
	t := template.Must(template.ParseFS(
		templates,
		"templates/rooms.tmpl",
	))
	t.ExecuteTemplate(w, "device", device)
}

func (s *server) addSubscriber(subscriber *subscriber) {
	s.subscriberMutex.Lock()
	s.subscribers[*subscriber] = true
	s.subscriberMutex.Unlock()
	log.Printf("added subscriber: %s", subscriber.ip)
}

func (s *server) removeSubscriber(subscriber *subscriber) {
	s.subscriberMutex.Lock()
	delete(s.subscribers, *subscriber)
	s.subscriberMutex.Unlock()
	log.Printf("removed subscriber: %s", subscriber.ip)
}

func (s *server) subscribe(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var c *websocket.Conn
	subscriber := &subscriber{
		messages: make(chan *Message),
		ip:       r.RemoteAddr,
	}
	s.addSubscriber(subscriber)
	defer s.removeSubscriber(subscriber)

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return err
	}
	defer c.CloseNow()

	ctx = c.CloseRead(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-subscriber.messages:
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			jsonMsg, err := json.Marshal(msg)
			if err != nil {
				return err
			}
			if err := c.Write(ctx, websocket.MessageText, jsonMsg); err != nil {
				return err
			}
		}
	}
}

func (s *server) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	err := s.subscribe(r.Context(), w, r)
	if errors.Is(err, context.Canceled) {
		return
	}
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return
	}
	if err != nil {
		log.Printf("error: %v", err)
		return
	}
}

func (s *server) broadcast(msg *Message) {
	log.Printf("broadcasting message: %+v", msg)
	s.subscriberMutex.Lock()
	defer s.subscriberMutex.Unlock()
	for subscriber := range s.subscribers {
		subscriber.messages <- msg
	}
}

func NewServer(messages chan *Message) *server {
	// mux := http.NewServeMux()
	nexa := &Nexa{
		Config: NewNexaConfig(),
	}

	s := &server{
		nexa:            nexa,
		mux:             http.NewServeMux(),
		subscribers:     make(map[subscriber]bool),
		subscriberMutex: sync.Mutex{},
	}

	// Pages
	s.mux.HandleFunc("GET /{$}", s.roomsHandler)

	// API
	s.mux.HandleFunc("GET /devices/{id}", s.deviceHandler)

	// Websockets
	s.mux.HandleFunc("GET /ws", s.subscribeHandler)

	// Static files
	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("GET /", fs)

	go func(messages chan *Message) {
		for msg := range messages {
			log.Printf("%d subscribers", len(s.subscribers))
			s.broadcast(msg)
		}
	}(messages)

	return s
}
