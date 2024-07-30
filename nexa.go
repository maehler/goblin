package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maehler/goblin/auth"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
)

type Nexa struct {
	Config *NexaConfig
}

type NexaConfig struct {
	URL      url.URL
	Username string
	Password string
}

type NexaNodes []NexaNode

type NexaNode struct {
	Id           string               `json:"id"`
	Name         string               `json:"name"`
	Capabilities []string             `json:"capabilities"`
	LastEvents   map[string]NexaEvent `json:"lastEvents"`
}

type NexaEvent struct {
	Name      string      `json:"name"`
	Value     interface{} `json:"value"`
	PrevValue interface{} `json:"prevValue"`
	Time      time.Time   `json:"time"`
}

type NexaRooms []NexaRoom

type NexaRoom struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	TempSensor      string `json:"tempSensor"`
	BackgroundImage string `json:"backURL"`
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

func (n *Nexa) Nodes() (*NexaNodes, error) {
	client := &http.Client{}
	nodesURL := n.Config.URL
	nodesURL.Path = "v1/nodes"

	da := auth.NewDigestAuth(n.Config.Username, n.Config.Password)
	req, err := da.Request("GET", nodesURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Println(resp.Status)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	nodes := &NexaNodes{}
	if err := json.Unmarshal(body, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (n *Nexa) Node(nodeId string) (*NexaNode, error) {
	client := &http.Client{}
	nodesURL := n.Config.URL
	nodesURL.Path = fmt.Sprintf("v1/nodes/%s", nodeId)

	da := auth.NewDigestAuth(n.Config.Username, n.Config.Password)
	req, err := da.Request("GET", nodesURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Println(resp.Status)

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
	return node, nil
}

func (n *Nexa) Rooms() (*NexaRooms, error) {
	client := &http.Client{}
	roomsURL := n.Config.URL
	roomsURL.Path = "v1/rooms"

	da := auth.NewDigestAuth(n.Config.Username, n.Config.Password)
	req, err := da.Request("GET", roomsURL.String())
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Println(resp.Status)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rooms := &NexaRooms{}
	if err := json.Unmarshal(body, rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

func InitSockets(host string, port int, messages chan string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s:%d", host, port), nil)
	if err != nil {
		panic(err)
	}
	defer c.CloseNow()

	for {
		_, r, err := c.Reader(context.TODO())
		if err != nil {
			log.Println("error reading from socket:", err.Error())
			close(messages)
			return
		}
		b, err := io.ReadAll(r)
		if err != nil {
			log.Println("error reading message:", err.Error())
			close(messages)
			return
		}
		messages <- string(b)
	}
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

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
	defer pc.Close()

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
	SystemType string      `json:"systemType"`
	Subtype    string      `json:"subtype"`
	SourceNode string      `json:"sourceNode"`
	Capability string      `json:"capability"`
	Name       string      `json:"name"`
	Value      interface{} `json:"value"`
	Time       time.Time   `json:"time"`
	Event      string      `json:"event"`
	NodeId     string      `json:"nodeId"`
}

func ParseMessage(message string) (*Message, error) {
	switch {
	case strings.HasPrefix(message, "temperature:"):
		message = strings.TrimPrefix(message, "temperature:")
	case strings.HasPrefix(message, "humidity:"):
		message = strings.TrimPrefix(message, "humidity:")
	case strings.HasPrefix(message, "nodeManager:"):
		message = strings.TrimPrefix(message, "nodeManager:")
	}

	m := &Message{}
	err := json.Unmarshal([]byte(message), m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func MessageConsumer(inputMessages chan string, outputMessages chan *Message) {
	for msg := range inputMessages {
		m, err := ParseMessage(msg)
		if err != nil {
			log.Printf("error parsing message: %s: %s", err.Error(), msg)
			continue
		}
		outputMessages <- m
	}
	close(outputMessages)
}
