package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Helper functions to set up test containers

func startRedisContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "redis:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)
	return container, fmt.Sprintf("%s:%s", host, port.Port())
}

func startRabbitMQContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:3-management",
		ExposedPorts: []string{"5672/tcp"},
		WaitingFor:   wait.ForLog("Server startup complete"),
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": "guest",
			"RABBITMQ_DEFAULT_PASS": "guest",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5672")
	require.NoError(t, err)

	return container, fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port())
}

func setupRabbitMQChannel(t *testing.T, rabbitmqAddr string) (*amqp091.Connection, *amqp091.Channel) {
	t.Helper()

	// Try to connect with retries

	var (
		conn *amqp091.Connection
		err  error
	)

	for i := 0; i < 5; i++ {
		conn, err = amqp091.Dial(rabbitmqAddr)
		if err == nil {
			break
		}
		t.Logf("Failed to connect to RabbitMQ, retrying in 2 seconds: %v", err)
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err)

	ch, err := conn.Channel()
	require.NoError(t, err)
	return conn, ch
}

// Helper function to clean Redis between tests
func cleanRedis(t *testing.T, redisAddr string) {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer client.Close()

	err := client.FlushAll(context.Background()).Err()
	require.NoError(t, err)
}
