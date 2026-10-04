package websocket_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", host, port.Port())})

	return client, func() {
		_ = client.Close()
		_ = container.Terminate(ctx)
	}
}

func TestHubPublishToUserDeliversViaRedisSubscriber(t *testing.T) {
	rdb, cleanup := setupRedis(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := wsplatform.NewHub(rdb, zerolog.Nop())
	require.NoError(t, hub.StartSubscriber(ctx))
	recorder := &recordingClient{}
	hub.RegisterRecorder("user-1", recorder)

	env, err := wsplatform.NewEnvelope(wsplatform.EventNewMessage, map[string]string{"body": "hi"})
	require.NoError(t, err)
	require.NoError(t, hub.PublishToUser(ctx, "user-1", env))
	require.Eventually(t, func() bool { return recorder.count() == 1 }, 2*time.Second, 50*time.Millisecond)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(recorder.first().Payload, &payload))
	require.Equal(t, "hi", payload["body"])
}

func TestHubPublishToUserDoesNotCrossDeliver(t *testing.T) {
	rdb, cleanup := setupRedis(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := wsplatform.NewHub(rdb, zerolog.Nop())
	require.NoError(t, hub.StartSubscriber(ctx))
	recorderA := &recordingClient{}
	recorderB := &recordingClient{}
	hub.RegisterRecorder("user-a", recorderA)
	hub.RegisterRecorder("user-b", recorderB)

	env, err := wsplatform.NewEnvelope(wsplatform.EventNewMessage, map[string]string{"body": "only A"})
	require.NoError(t, err)
	require.NoError(t, hub.PublishToUser(ctx, "user-a", env))
	require.Eventually(t, func() bool { return recorderA.count() == 1 }, 2*time.Second, 50*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	require.Zero(t, recorderB.count())
}
