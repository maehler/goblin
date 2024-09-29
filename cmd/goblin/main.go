package main

import (
	"log"
	"os"

	"github.com/maehler/goblin/http"
	"github.com/maehler/goblin/nexa"
	"github.com/maehler/goblin/sqlite"
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
	viper.SetDefault("home_name", "goblin")

	viper.SetEnvPrefix("goblin")
	viper.MustBindEnv("home_name")
	viper.MustBindEnv("host")
	viper.MustBindEnv("port")

	nexaIP, err := nexa.IdentifyNexa()
	if err != nil {
		return err
	}
	log.Printf("detected Nexa at %s", nexaIP)
	viper.SetDefault("nexa.address", nexaIP)

	return viper.ReadInConfig()
}

// TODO: save temperature and humidity to the database

func main() {
	if err := config(); err != nil {
		log.Fatal(err)
	}
	log.Printf("using config file %s", viper.ConfigFileUsed())
	db := sqlite.NewDatabase(viper.GetString("sqlite_dsn"))
	if err := db.Open(); err != nil {
		log.Fatal(err)
	}

	log.Printf("connecting to Nexa at %s", viper.GetString("nexa.address"))

	nexaConfig := nexa.NewNexaConfig()
	nexaConfig.Username = viper.GetString("nexa.username")
	nexaConfig.Password = viper.GetString("nexa.password")
	nexaConfig.WebsocketHost = viper.GetString("nexa.address")
	nexaConfig.WebsocketPort = viper.GetInt("nexa.socket_port")
	nxa := nexa.NewNexa(nexaConfig)
	go nxa.InitSockets()

	server := http.NewServer(
		http.WithName(viper.GetString("home_name")),
		http.WithHost(viper.GetString("host")),
		http.WithPort(viper.GetInt("port")),
	)

	server.RoomService = sqlite.NewRoomService(db)
	server.SensorService = sqlite.NewSensorService(db)
	server.NexaService = nexa.NewNexaService(nxa)

	server.Serve()
}
