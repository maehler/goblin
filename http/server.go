package http

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/maehler/goblin"
	"github.com/maehler/goblin/nexa"
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

func newTemplateHandler(fs fs.FS, name string) *templateHandler {
	t := template.New("")
	t.Funcs(template.FuncMap{
		"has":      hasString,
		"homeName": func() string { return name },
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
	host            string
	port            int
	mux             *http.ServeMux
	subscriberMutex sync.Mutex
	subscribers     map[subscriber]bool
	*templateHandler

	RoomService   goblin.RoomService
	SensorService goblin.SensorService
	NexaService   nexa.NexaService
}

func hasString(slice []string, value string) bool {
	return slices.Contains(slice, value)
}

func (s *server) roomsHandler(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.NexaService.Rooms()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := fmt.Fprintf(w, "error: %+v", err); err != nil {
			slog.Error("failed to write error in response", "error", err)
		}
		return
	}
	if err := s.templates.ExecuteTemplate(w, "layout.tmpl", M{"rooms": rooms, "time": time.Now()}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error("error executing template", "error", err.Error())
	}
}

func (s *server) deviceHandler(w http.ResponseWriter, r *http.Request) {
	device, err := s.NexaService.Node(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := fmt.Fprintf(w, "error: %+v", err); err != nil {
			slog.Error("failed to write error in response", "error", err)
		}
		return
	}
	if err := s.templates.ExecuteTemplate(w, "device", device); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error("error executing template", "error", err.Error())
	}
}

func (s *server) addSubscriber(subscriber *subscriber) {
	s.subscriberMutex.Lock()
	s.subscribers[*subscriber] = true
	s.subscriberMutex.Unlock()
	slog.Info("added subscriber", "ip", subscriber.ip)
}

func (s *server) removeSubscriber(subscriber *subscriber) {
	s.subscriberMutex.Lock()
	delete(s.subscribers, *subscriber)
	s.subscriberMutex.Unlock()
	slog.Info("removed subscriber", "ip", subscriber.ip)
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
	defer func() {
		if err := c.CloseNow(); err != nil {
			slog.Error("failed to close websocket", "error", err)
		}
	}()

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
		slog.Error("websocket subscribe failed", "error", err)
		return
	}
}

func (s *server) broadcast(msg *nexa.Message) error {
	var useTemplate string
	if msg.Capability != "" {
		useTemplate = msg.Capability
	} else if msg.SystemType == "time" {
		useTemplate = msg.Subtype
	}

	if !s.HasTemplate(useTemplate) {
		slog.Error("template not found, ignoring message", "template", useTemplate, "full_message", msg)
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

type options struct {
	name *string
	host *string
	port *int
}

type Option func(*options) error

func WithName(name string) Option {
	return func(options *options) error {
		if len(name) > 128 {
			return fmt.Errorf("home name cannot be longer than 128 characters")
		}
		if len(name) == 0 {
			return fmt.Errorf("home name cannot be empty")
		}
		options.name = &name
		return nil
	}
}

func WithHost(host string) Option {
	return func(options *options) error {
		options.host = &host
		return nil
	}
}

func WithPort(port int) Option {
	return func(options *options) error {
		if port < 80 || port > 65535 {
			return fmt.Errorf("port must be between 80 and 65535")
		}
		options.port = &port
		return nil
	}
}

func NewServer(opts ...Option) *server {
	options := options{}
	for _, o := range opts {
		if err := o(&options); err != nil {
			slog.Error("failed to process options", "error", err)
		}
	}

	var name string
	if options.name != nil {
		name = *options.name
	} else {
		name = "goblin"
	}

	slog.Info("setting home name", "name", name)

	var host string
	if options.host != nil {
		host = *options.host
	} else {
		name = "localhost"
	}

	var port int
	if options.port != nil {
		port = *options.port
	} else {
		port = 3000
	}

	s := &server{
		host:            host,
		port:            port,
		mux:             http.NewServeMux(),
		subscribers:     make(map[subscriber]bool),
		subscriberMutex: sync.Mutex{},
		templateHandler: newTemplateHandler(templateFS, name),
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
		slog.Error("failed to set up static file system", "error", err)
		panic(err)
	}
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("GET /", fs)

	return s
}

func (s *server) Serve() error {
	slog.Info("starting server", "host", s.host, "port", s.port)

	if s.NexaService.Nexa != nil {
		go func(messages chan nexa.Message) {
			for msg := range messages {
				slog.Info("broadcasting subscribers", "count", len(s.subscribers), "message", msg)
				if err := s.broadcast(&msg); err != nil {
					slog.Error("failed to broadcast", "error", err)
				}
			}
		}(s.NexaService.Nexa.Messages)
	}

	// Setting up rooms and sensors in the database
	rooms, err := s.NexaService.Rooms()
	if err != nil {
		return err
	}
	for _, room := range rooms {
		r := goblin.NewRoom(room.Id, room.Name)
		if err := s.RoomService.CreateRoom(context.Background(), &r); err != nil {
			slog.Error("failed to create room", "error", err)
		}
	}

	return http.ListenAndServe(
		fmt.Sprintf(
			"%s:%d",
			s.host,
			s.port,
		),
		s.mux,
	)
}
