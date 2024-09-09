package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/spf13/viper"
	"nhooyr.io/websocket"
)

type M = map[string]any

//go:embed templates
var templateFS embed.FS

//go:embed all:static
var static embed.FS

type subscriber struct {
	messages chan string
	ip       string
}

type templateHandler struct {
	templates *template.Template
}

func newTemplateHandler(fs fs.FS) *templateHandler {
	t := template.New("")
	t.Funcs(template.FuncMap{
		"has":      hasString,
		"homeName": homeName,
	})
	t = template.Must(t.ParseFS(fs, "templates/*.tmpl"))
	return &templateHandler{t}
}

func (t templateHandler) HasTemplate(name string) bool {
	for _, t := range t.templates.Templates() {
		if t.Name() == name {
			return true
		}
	}
	return false
}

type server struct {
	nexa            *Nexa
	mux             *http.ServeMux
	subscriberMutex sync.Mutex
	subscribers     map[subscriber]bool
	*templateHandler
}

func hasString(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func homeName() string {
	return viper.GetString("home_name")
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
	if err := s.templates.ExecuteTemplate(w, "layout.tmpl", M{"rooms": rooms, "time": time.Now()}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("error executing template:", err.Error())
	}
}

func (s *server) deviceHandler(w http.ResponseWriter, r *http.Request) {
	device, err := s.nexa.Node(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("error: %+v", err.Error())))
		return
	}
	if err := s.templates.ExecuteTemplate(w, "device", device); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("error executing template: %s", err.Error())
	}
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
		messages: make(chan string),
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
			if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
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

func (s *server) broadcast(msg *Message) error {
	var useTemplate string
	if msg.Capability != "" {
		useTemplate = msg.Capability
	} else if msg.SystemType == "time" {
		useTemplate = msg.Subtype
	}

	if !s.HasTemplate(useTemplate) {
		log.Printf("template %q not found, ignoring message. Full message was: %#v", useTemplate, msg)
		return nil
	}

	var htmlMsg bytes.Buffer
	if err := s.templates.ExecuteTemplate(&htmlMsg, useTemplate, msg); err != nil {
		return err
	}

	s.subscriberMutex.Lock()
	defer s.subscriberMutex.Unlock()
	for subscriber := range s.subscribers {
		subscriber.messages <- htmlMsg.String()
	}

	return nil
}

func NewServer(messages chan *Message) *server {
	nexa := &Nexa{
		Config: NewNexaConfig(),
	}

	s := &server{
		nexa:            nexa,
		mux:             http.NewServeMux(),
		subscribers:     make(map[subscriber]bool),
		subscriberMutex: sync.Mutex{},
		templateHandler: newTemplateHandler(templateFS),
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
			log.Printf("broadcasting to %d subscribers: %s", len(s.subscribers), msg)
			if err := s.broadcast(msg); err != nil {
				log.Println("broadcast error:", err.Error())
			}
		}
	}(messages)

	return s
}
