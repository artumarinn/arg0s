package telemetry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/storage"
)

func TestBus_PublishNotifiesSubscribers(t *testing.T) {
	bus := NewBus(nil)
	ch := bus.Subscribe()

	err := bus.Publish(context.Background(), Event{Type: EventTaskCreated})
	require.NoError(t, err)

	select {
	case ev := <-ch:
		require.Equal(t, EventTaskCreated, ev.Type)
		require.NotEmpty(t, ev.ID)
	default:
		t.Fatal("suscriptor no recibió el evento")
	}
}

func TestBus_PersistsToSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arg0s.db")
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()

	bus := NewBus(db.DB)
	err = bus.Publish(context.Background(), Event{
		Type:      EventTaskCreated,
		SessionID: "sess_1",
		Payload:   map[string]any{"foo": "bar"},
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM events WHERE type = ?`, string(EventTaskCreated)).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
