package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Device   DeviceConfig
	Database DatabaseConfig
	JWT      JWTConfig
	WS       WSConfig
	Update   UpdateConfig
}

type ServerConfig struct {
	Port int
	Mode string
}

type DeviceConfig struct {
	Limit int
}

type DatabaseConfig struct {
	Host           string
	Port           int
	User           string
	Password       string
	DBName         string
	SSLMode        string
	ConnectTimeout time.Duration
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type WSConfig struct {
	TicketTTL       time.Duration
	TicketRateLimit int
	MaxMessageBytes int64
}

type UpdateConfig struct {
	CheckInterval time.Duration
	StoragePath   string
	GitHub        GitHubConfig
	Proxy         ProxyConfig
}

type ProxyConfig struct {
	HTTP    string
	HTTPS   string
	NoProxy string
}

type GitHubConfig struct {
	Token string
	Repos []GitHubRepoConfig
}

type GitHubRepoConfig struct {
	Owner    string
	Repo     string
	Platform string
}

// env helpers

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %q", key, raw)
	}
	return value, nil
}

func envPositiveInt(key string, fallback int) (int, error) {
	value, err := envInt(key, fallback)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func envPositiveInt64(key string, fallback int64) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %q", key, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}

// parseReposEnv parses "owner:repo:platform,owner:repo:platform" into repo configs.
func parseReposEnv(raw string) ([]GitHubRepoConfig, error) {
	var repos []GitHubRepoConfig
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid repo entry %q, expected owner:repo:platform", entry)
		}
		repos = append(repos, GitHubRepoConfig{
			Owner:    parts[0],
			Repo:     parts[1],
			Platform: parts[2],
		})
	}
	return repos, nil
}

func Load() (*Config, error) {
	var configErrors []error
	collect := func(err error) {
		if err != nil {
			configErrors = append(configErrors, err)
		}
	}

	serverPort, err := envInt("COLINK_SERVER_PORT", 8080)
	collect(err)
	if err == nil && (serverPort < 1 || serverPort > 65535) {
		collect(fmt.Errorf("COLINK_SERVER_PORT must be between 1 and 65535"))
	}
	databasePort, err := envInt("COLINK_DATABASE_PORT", 5432)
	collect(err)
	if err == nil && (databasePort < 1 || databasePort > 65535) {
		collect(fmt.Errorf("COLINK_DATABASE_PORT must be between 1 and 65535"))
	}
	deviceLimit, err := envPositiveInt("COLINK_DEVICE_LIMIT", 20)
	collect(err)
	databaseConnectTimeout, err := envDuration("COLINK_DATABASE_CONNECT_TIMEOUT", 2*time.Minute)
	collect(err)
	accessTTL, accessTTLErr := envDuration("COLINK_JWT_ACCESS_TTL", 72*time.Hour)
	collect(accessTTLErr)
	refreshTTL, refreshTTLErr := envDuration("COLINK_JWT_REFRESH_TTL", 30*24*time.Hour)
	collect(refreshTTLErr)
	if accessTTLErr == nil && refreshTTLErr == nil && accessTTL > refreshTTL {
		collect(fmt.Errorf("COLINK_JWT_ACCESS_TTL must not exceed COLINK_JWT_REFRESH_TTL"))
	}
	ticketTTL, err := envDuration("COLINK_WS_TICKET_TTL", 30*time.Second)
	collect(err)
	ticketRateLimit, err := envPositiveInt("COLINK_WS_TICKET_RATE_LIMIT", 20)
	collect(err)
	maxMessageBytes, err := envPositiveInt64("COLINK_WS_MAX_MESSAGE_BYTES", 8*1024*1024)
	collect(err)
	updateCheckInterval, err := envDuration("COLINK_UPDATE_CHECK_INTERVAL", 30*time.Minute)
	collect(err)

	var repos []GitHubRepoConfig
	if raw := os.Getenv("COLINK_UPDATE_GITHUB_REPOS"); raw != "" {
		repos, err = parseReposEnv(raw)
		if err != nil {
			collect(fmt.Errorf("parse COLINK_UPDATE_GITHUB_REPOS: %w", err))
		}
	}

	if len(configErrors) > 0 {
		return nil, errors.Join(configErrors...)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: serverPort,
			Mode: env("COLINK_SERVER_MODE", "debug"),
		},
		Device: DeviceConfig{
			Limit: deviceLimit,
		},
		Database: DatabaseConfig{
			Host:           env("COLINK_DATABASE_HOST", "localhost"),
			Port:           databasePort,
			User:           env("COLINK_DATABASE_USER", "colink"),
			Password:       env("COLINK_DATABASE_PASSWORD", ""),
			DBName:         env("COLINK_DATABASE_DBNAME", "colink"),
			SSLMode:        env("COLINK_DATABASE_SSLMODE", "disable"),
			ConnectTimeout: databaseConnectTimeout,
		},
		JWT: JWTConfig{
			Secret:     env("COLINK_JWT_SECRET", ""),
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		WS: WSConfig{
			TicketTTL:       ticketTTL,
			TicketRateLimit: ticketRateLimit,
			MaxMessageBytes: maxMessageBytes,
		},
		Update: UpdateConfig{
			CheckInterval: updateCheckInterval,
			StoragePath:   env("COLINK_UPDATE_STORAGE_PATH", "./data/updates"),
			GitHub: GitHubConfig{
				Token: env("COLINK_UPDATE_GITHUB_TOKEN", ""),
				Repos: repos,
			},
			Proxy: ProxyConfig{
				HTTP:    env("COLINK_HTTP_PROXY", ""),
				HTTPS:   env("COLINK_HTTPS_PROXY", ""),
				NoProxy: env("COLINK_NO_PROXY", ""),
			},
		},
	}

	return cfg, nil
}

func (c DatabaseConfig) DSN() string {
	return c.DSNForDatabase(c.DBName)
}

func (c DatabaseConfig) DSNForDatabase(databaseName string) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		c.Host,
		c.Port,
		c.User,
		c.Password,
		databaseName,
		c.SSLMode,
	)
}
