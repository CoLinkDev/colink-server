package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	WS       WSConfig
	Update   UpdateConfig
}

type ServerConfig struct {
	Port int
	Mode string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type WSConfig struct {
	TicketTTL time.Duration
}

type UpdateConfig struct {
	CheckInterval time.Duration
	StoragePath   string
	GitHub        GitHubConfig
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

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
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
	cfg := &Config{
		Server: ServerConfig{
			Port: envInt("SERVER_PORT", 8080),
			Mode: env("SERVER_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     env("DATABASE_HOST", "localhost"),
			Port:     envInt("DATABASE_PORT", 5432),
			User:     env("DATABASE_USER", "colink"),
			Password: env("DATABASE_PASSWORD", ""),
			DBName:   env("DATABASE_DBNAME", "colink"),
			SSLMode:  env("DATABASE_SSLMODE", "disable"),
		},
		JWT: JWTConfig{
			Secret:     env("JWT_SECRET", ""),
			AccessTTL:  envDuration("JWT_ACCESS_TTL", 72*time.Hour),
			RefreshTTL: envDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		},
		WS: WSConfig{
			TicketTTL: envDuration("WS_TICKET_TTL", 30*time.Second),
		},
		Update: UpdateConfig{
			CheckInterval: envDuration("UPDATE_CHECK_INTERVAL", 30*time.Minute),
			StoragePath:   env("UPDATE_STORAGE_PATH", "./data/updates"),
			GitHub: GitHubConfig{
				Token: env("UPDATE_GITHUB_TOKEN", ""),
			},
		},
	}

	if raw := os.Getenv("UPDATE_GITHUB_REPOS"); raw != "" {
		repos, err := parseReposEnv(raw)
		if err != nil {
			return nil, fmt.Errorf("parse UPDATE_GITHUB_REPOS: %w", err)
		}
		cfg.Update.GitHub.Repos = repos
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
