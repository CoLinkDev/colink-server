package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	WS       WSConfig       `mapstructure:"ws"`
	Update   UpdateConfig   `mapstructure:"update"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type WSConfig struct {
	TicketTTL time.Duration `mapstructure:"ticket_ttl"`
}

type UpdateConfig struct {
	CheckInterval time.Duration `mapstructure:"check_interval"`
	StoragePath   string        `mapstructure:"storage_path"`
	GitHub        GitHubConfig  `mapstructure:"github"`
}

type GitHubConfig struct {
	Token string             `mapstructure:"token"`
	Proxy string             `mapstructure:"proxy"`
	Repos []GitHubRepoConfig `mapstructure:"repos"`
}

type GitHubRepoConfig struct {
	Owner    string `mapstructure:"owner"`
	Repo     string `mapstructure:"repo"`
	Platform string `mapstructure:"platform"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnv(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	v.SetConfigFile("config.local.yaml")
	if err := v.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config.local.yaml: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		)
	}); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}
	if cfg.JWT.AccessTTL == 0 {
		cfg.JWT.AccessTTL = 15 * time.Minute
	}
	if cfg.JWT.RefreshTTL == 0 {
		cfg.JWT.RefreshTTL = 30 * 24 * time.Hour
	}
	if cfg.WS.TicketTTL == 0 {
		cfg.WS.TicketTTL = 30 * time.Second
	}
	if cfg.Update.CheckInterval == 0 {
		cfg.Update.CheckInterval = 30 * time.Minute
	}
	if cfg.Update.StoragePath == "" {
		cfg.Update.StoragePath = "./data/updates"
	}

	return &cfg, nil
}

func bindEnv(v *viper.Viper) {
	keys := map[string]string{
		"server.port":           "SERVER_PORT",
		"server.mode":           "SERVER_MODE",
		"database.host":         "DATABASE_HOST",
		"database.port":         "DATABASE_PORT",
		"database.user":         "DATABASE_USER",
		"database.password":     "DATABASE_PASSWORD",
		"database.dbname":       "DATABASE_DBNAME",
		"database.sslmode":      "DATABASE_SSLMODE",
		"jwt.secret":            "JWT_SECRET",
		"jwt.access_ttl":        "JWT_ACCESS_TTL",
		"jwt.refresh_ttl":       "JWT_REFRESH_TTL",
		"ws.ticket_ttl":         "WS_TICKET_TTL",
		"update.check_interval": "UPDATE_CHECK_INTERVAL",
		"update.storage_path":   "UPDATE_STORAGE_PATH",
		"update.github.token":   "UPDATE_GITHUB_TOKEN",
		"update.github.proxy":   "UPDATE_GITHUB_PROXY",
	}

	for key, env := range keys {
		_ = v.BindEnv(key, env)
	}
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		c.Host,
		c.Port,
		c.User,
		c.Password,
		c.DBName,
		c.SSLMode,
	)
}
