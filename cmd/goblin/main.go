package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/maehler/goblin/http"
	"github.com/maehler/goblin/nexa"
	"github.com/maehler/goblin/sqlite"
	"github.com/spf13/viper"
)

func config() error {
	viper.SetConfigName("goblin")
	viper.SetConfigType("yaml")
	homedir, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(homedir)
	} else {
		slog.Error("failed to get home directory", "error", err)
	}
	if _, ok := os.LookupEnv("XDG_CONFIG"); ok {
		viper.AddConfigPath("XDG_CONFIG")
	}
	viper.AddConfigPath(filepath.Join(homedir, ".config"))
	viper.AddConfigPath("/etc")
	viper.AddConfigPath(".")

	viper.SetDefault("host", "0.0.0.0")
	viper.SetDefault("port", 8080)
	viper.SetDefault("nexa.socket_port", 8887)
	viper.SetDefault("nexa.username", "nexa")
	viper.SetDefault("nexa.password", "nexa")
	viper.SetDefault("home_name", "goblin")

	dataPath, ok := os.LookupEnv("XDG_DATA_HOME")
	if !ok {
		dataPath = filepath.Join(homedir, ".local", "share")
	}
	dataPath = filepath.Join(dataPath, "goblin")
	if _, err := os.Stat(dataPath); err == os.ErrNotExist {
		if err := os.MkdirAll(dataPath, 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	viper.SetDefault("sqlite_dsn", "file:"+filepath.Join(dataPath, "goblin.db"))

	viper.SetEnvPrefix("goblin")
	viper.MustBindEnv("home_name")
	viper.MustBindEnv("host")
	viper.MustBindEnv("port")
	viper.MustBindEnv("sqlite_dsn")
	viper.MustBindEnv("loglevel")

	viper.SetDefault("loglevel", "warning")
	loglevel := viper.GetString("loglevel")
	switch strings.ToLower(loglevel) {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "warn", "warning":
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	default:
		return fmt.Errorf("invalid log level: %s", loglevel)
	}

	nexaIP, err := nexa.IdentifyNexa()
	if err != nil {
		return err
	}
	slog.Info("detected nexa", "address", nexaIP)
	viper.SetDefault("nexa.address", nexaIP)

	return viper.ReadInConfig()
}

// TODO: save temperature and humidity to the database

func main() {
	if err := config(); err != nil {
		slog.Error("failed to initialize config", "error", err)
		os.Exit(1)
	}
	slog.Info("config file", "path", viper.ConfigFileUsed())
	db := sqlite.NewDatabase(viper.GetString("sqlite_dsn"))
	if err := db.Open(); err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	slog.Info("connecting to nexa", "address", viper.GetString("nexa.address"))

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

	if err := server.Serve(); err != nil {
		slog.Error("server crashed", "error", err)
		os.Exit(1)
	}
}
