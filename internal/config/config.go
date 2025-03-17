package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config holds all the configuration settings for the application
type Config struct {
	// RabbitMQ configuration
	RabbitMQURL string `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@localhost:5672/"`
	QueueName   string `env:"QUEUE_NAME" envDefault:"matchmaking_queue"`

	// Redis configuration
	RedisAddr string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPass string `env:"REDIS_PASS" envDefault:""`
	RedisDB   int    `env:"REDIS_DB" envDefault:"0"`

	// Director configuration
	MatchInterval time.Duration `env:"MATCH_INTERVAL" envDefault:"5s"`

	// Logging configuration
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load returns the configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}
