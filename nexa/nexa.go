package nexa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/maehler/goblin/auth"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
)

type NexaService struct {
	Nexa *Nexa
}

func NewNexaService(nexa *Nexa) NexaService {
	return NexaService{
		Nexa: nexa,
	}
}

type NexaSensor interface {
	Id() (string, error)
	BoolValue() (bool, error)
	FloatValue() (float64, error)
	IntValue() (int, error)
	StringValue() string
}

type Nexa struct {
	// Nexa config
	Config *NexaConfig

	// Parsed output messages
	Messages chan Message
}

func NewNexa(config *NexaConfig) *Nexa {
	return &Nexa{
		Config:   config,
		Messages: make(chan Message),
	}
}

type NexaConfig struct {
	URL           url.URL
	Username      string
	Password      string
	WebsocketHost string
	WebsocketPort int
}

type NexaNodes = []*NexaNode

type NexaNode struct {
	Id           string                `json:"id"`
	Name         string                `json:"name"`
	RoomId       string                `json:"roomId"`
	Capabilities []string              `json:"capabilities"`
	LastEvents   map[string]*NexaEvent `json:"lastEvents"`
}

type NexaEvent struct {
	NodeId    string
	Name      string    `json:"name"`
	Value     any       `json:"value"`
	PrevValue any       `json:"prevValue"`
	Time      time.Time `json:"time"`
}

func (n NexaEvent) Id() string {
	return n.NodeId
}

func (n NexaEvent) BoolValue() (bool, error) {
	v, ok := n.Value.(bool)
	if !ok {
		return false, fmt.Errorf("invalid bool value: %v", n.Value)
	}
	return v, nil
}

func (n NexaEvent) FloatValue() (float64, error) {
	v, ok := n.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid float value: %v", n.Value)
	}
	return v, nil
}

func (n NexaEvent) IntValue() (int, error) {
	v, ok := n.Value.(int)
	if !ok {
		return 0, fmt.Errorf("invalid int value: %v", n.Value)
	}
	return v, nil
}

func (n NexaEvent) StringValue() string {
	return fmt.Sprintf("%v", n.Value)
}

type NexaRooms = []NexaRoom

type NexaRoom struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	TempSensor      string `json:"tempSensor"`
	BackgroundImage string `json:"backURL"`
	Nodes           NexaNodes
}

func NewNexaConfig() *NexaConfig {
	return &NexaConfig{
		URL: url.URL{
			Scheme: "http",
			Host:   viper.GetString("nexa.address"),
		},
		Username: viper.GetString("nexa.username"),
		Password: viper.GetString("nexa.password"),
	}
}

func (s *NexaService) Nodes() (NexaNodes, error) {
	client := &http.Client{}
	nodesURL := s.Nexa.Config.URL
	nodesURL.Path = "v1/nodes"

	da := auth.NewDigestAuth(s.Nexa.Config.Username, s.Nexa.Config.Password)
	req, err := da.Request("GET", nodesURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}()

	slog.Info("nodes response", "status", resp.Status)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	nodes := NexaNodes{}
	if err := json.Unmarshal(body, &nodes); err != nil {
		return nil, err
	}
	for _, node := range nodes {
		for _, event := range node.LastEvents {
			event.NodeId = node.Id
		}
	}
	return nodes, nil
}

func (s *NexaService) Node(nodeId string) (*NexaNode, error) {
	client := &http.Client{}
	nodesURL := s.Nexa.Config.URL
	nodesURL.Path = fmt.Sprintf("v1/nodes/%s", nodeId)

	da := auth.NewDigestAuth(s.Nexa.Config.Username, s.Nexa.Config.Password)
	req, err := da.Request("GET", nodesURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}()

	slog.Info("node response", "status", resp.Status)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	node := &NexaNode{}
	if err := json.Unmarshal(body, node); err != nil {
		return nil, err
	}
	for _, event := range node.LastEvents {
		event.NodeId = node.Id
	}
	return node, nil
}

func (s *NexaService) Rooms() (NexaRooms, error) {
	client := &http.Client{}
	roomsURL := s.Nexa.Config.URL
	roomsURL.Path = "v1/rooms"

	da := auth.NewDigestAuth(s.Nexa.Config.Username, s.Nexa.Config.Password)
	req, err := da.Request("GET", roomsURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}()

	slog.Info("rooms response", "status", resp.Status)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rooms := NexaRooms{}
	if err := json.Unmarshal(body, &rooms); err != nil {
		return nil, err
	}

	roomIds := make(map[string]int)
	for i, room := range rooms {
		roomIds[room.Id] = i
	}

	nodes, err := s.Nodes()
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.RoomId == "" {
			continue
		}
		roomIndex := roomIds[node.RoomId]
		rooms[roomIndex].Nodes = append(rooms[roomIndex].Nodes, node)
	}

	return rooms, nil
}

func (n *Nexa) InitSockets() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s:%d", n.Config.WebsocketHost, n.Config.WebsocketPort), nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := c.CloseNow(); err != nil {
			slog.Error("failed to close websocket connection", "error", err)
		}
	}()

	for {
		_, r, err := c.Reader(context.TODO())
		if err != nil {
			log.Println("error reading from socket:", err.Error())
			return
		}
		b, err := io.ReadAll(r)
		if err != nil {
			log.Println("error reading message:", err.Error())
			return
		}
		msg, err := ParseMessage(string(b))
		if err != nil {
			log.Println("error parsing message:", err.Error())
			return
		}

		n.Messages <- *msg
	}
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		slog.Error("failed to connect to google dns", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Error("failed to close upd connection", "error", err)
		}
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func IdentifyNexa() (nexaIp string, err error) {
	serverAddr, err := net.ResolveUDPAddr("udp4", "255.255.255.255:43233")
	if err != nil {
		return
	}

	ourAddr, err := net.ResolveUDPAddr("udp4", GetOutboundIP().String()+":43233")
	if err != nil {
		return
	}

	pc, err := net.ListenUDP("udp4", ourAddr)
	if err != nil {
		return
	}
	defer func() {
		if err := pc.Close(); err != nil {
			slog.Error("failed to close connection", "error", err)
		}
	}()

	_, err = pc.WriteToUDP([]byte("hello"), serverAddr)
	if err != nil {
		return
	}

	buf := make([]byte, 1024)
	_, addr, err := pc.ReadFromUDP(buf)
	if err != nil {
		return
	}

	return addr.IP.String(), nil
}

type Message struct {
	SystemType string    `json:"systemType"`
	Subtype    string    `json:"subtype"`
	SourceNode string    `json:"sourceNode"`
	Capability string    `json:"capability"`
	Name       string    `json:"name"`
	Value      any       `json:"value"`
	Time       time.Time `json:"time"`
	Event      string    `json:"event"`
	NodeId     string    `json:"nodeId"`
}

func (m Message) Id() string {
	return m.SourceNode
}

func (m Message) BoolValue() (bool, error) {
	v, ok := m.Value.(bool)
	if !ok {
		return false, fmt.Errorf("invalid bool value: %v", m.Value)
	}
	return v, nil
}

func (m Message) FloatValue() (float64, error) {
	v, ok := m.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid float value: %v", m.Value)
	}
	return v, nil
}

func (m Message) IntValue() (int, error) {
	v, ok := m.Value.(int)
	if !ok {
		return 0, fmt.Errorf("invalid int value: %v", m.Value)
	}
	return v, nil
}

func (m Message) StringValue() string {
	return fmt.Sprintf("%v", m.Value)
}

func (m *Message) String() string {
	s := ""
	if m.Name != "" {
		s += m.Name + " "
	}
	s += fmt.Sprintf("%s.%s: %v", m.SystemType, m.Subtype, m.Value)
	return s
}

func ParseMessage(message string) (*Message, error) {
	prefixRe := regexp.MustCompile("^[^{]+")
	message = prefixRe.ReplaceAllString(message, "")

	m := &Message{}
	err := json.Unmarshal([]byte(message), m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
