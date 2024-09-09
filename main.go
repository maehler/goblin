package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/viper"
)

func config() error {
	viper.SetConfigName("goblin")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME")
	if _, ok := os.LookupEnv("$XDG_CONFIG"); ok {
		viper.AddConfigPath("$XDG_CONFIG")
	}
	viper.AddConfigPath("$HOME/.config")
	viper.AddConfigPath("/etc")
	viper.AddConfigPath(".")

	viper.SetDefault("host", "0.0.0.0")
	viper.SetDefault("port", 8080)
	viper.SetDefault("nexa.socket_port", 8887)
	viper.SetDefault("nexa.username", "nexa")
	viper.SetDefault("nexa.password", "nexa")
	viper.SetDefault("home_name", "Goblin")

	nexaIP, err := IdentifyNexa()
	if err != nil {
		return err
	}
	log.Printf("Detected Nexa at %s", nexaIP)
	viper.SetDefault("nexa.address", nexaIP)

	return viper.ReadInConfig()
}

// TODO: save temperature and humidity to the database
// TODO: make a database interface

func main() {
	if err := config(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Using config file %s", viper.ConfigFileUsed())
	log.Printf("Connecting to Nexa at %s", viper.GetString("nexa.address"))
	stringMessages := make(chan string, 10)
	messages := make(chan *Message, 10)
	go InitSockets(
		viper.GetString("nexa.address"),
		viper.GetInt("nexa.socket_port"),
		stringMessages,
	)
	go MessageConsumer(stringMessages, messages)
	server := NewServer(messages)
	log.Printf("server running on %v:%v", viper.GetString("host"), viper.GetInt("port"))
	http.ListenAndServe(
		fmt.Sprintf(
			"%v:%v",
			viper.GetString("host"),
			viper.GetInt("port")),
		server.mux,
	)
}
