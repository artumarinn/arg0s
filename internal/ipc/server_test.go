package ipc

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

// startTestServer levanta un Server real sobre un Unix socket real en
// un directorio temporal -- esto es exactamente lo que valida el
// micro-checkpoint: streaming cruzando un socket de verdad, no un
// mock de la capa IPC.
func startTestServer(t *testing.T, providerCfg mock.Config) (socketPath string, client *Client) {
	t.Helper()
	providerCfg.Name = "mockprov"
	reg := providers.NewRegistry()
	reg.Register(mock.New(providerCfg))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{
		"m": {Provider: "mockprov", ProviderModelID: "m-1"},
	}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	srv := NewServer(exec, "")
	socketPath = filepath.Join(t.TempDir(), "arg0sd.sock")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.ListenAndServe(ctx, socketPath)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// esperar a que el socket exista -- el listener arranca en su
	// propia goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := Dial(socketPath); err == nil {
			return socketPath, c
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout esperando que arg0sd levante el socket")
	return "", nil
}

func TestPing_RespondsOverRealSocket(t *testing.T) {
	_, client := startTestServer(t, mock.Config{})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// TestSubmitAndSubscribe_ReceivesChunksAndExplicitDoneEvent es el
// micro-checkpoint: submit → subscribe → recibir el stream de eventos
// de una task real por el socket, terminando en un evento EXPLÍCITO
// (nunca inferido de que el socket se cerró).
func TestSubmitAndSubscribe_ReceivesChunksAndExplicitDoneEvent(t *testing.T) {
	_, client := startTestServer(t, mock.Config{Response: "hola mundo", ChunkDelay: time.Millisecond})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	taskID, err := client.Submit(ctx, "hola", "m")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var mu sync.Mutex
	var received []Event
	eventCh := make(chan struct{}, 1)
	if err := client.Subscribe(ctx, taskID, 0, func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		if e.Type != EventChunk {
			select {
			case eventCh <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	select {
	case <-eventCh:
	case <-time.After(3 * time.Second):
		t.Fatal("nunca llegó un evento terminal")
	}
	time.Sleep(20 * time.Millisecond) // margen para que el último Encode termine de flushear

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("no se recibió ningún evento")
	}
	last := received[len(received)-1]
	if last.Type != EventDone {
		t.Fatalf("último evento = %q, want done (explícito, no inferido)", last.Type)
	}

	var content string
	for _, e := range received {
		if e.Type == EventChunk {
			content += e.Delta
		}
	}
	if content != "hola mundo" {
		t.Fatalf("contenido reconstruido = %q, want %q", content, "hola mundo")
	}
}

// TestCancel_MidStream_ProducesExplicitCancelledEvent prueba la
// cancelación cruzando el socket: task.cancel corta una task viva y el
// cliente recibe un evento "cancelled" explícito, no un socket que se
// cierra sin más.
func TestCancel_MidStream_ProducesExplicitCancelledEvent(t *testing.T) {
	_, client := startTestServer(t, mock.Config{Response: "contenido largo para que dé tiempo a cancelar", ChunkDelay: 50 * time.Millisecond})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	taskID, err := client.Submit(ctx, "hola", "m")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	terminal := make(chan EventType, 1)
	if err := client.Subscribe(ctx, taskID, 0, func(e Event) {
		if e.Type != EventChunk {
			select {
			case terminal <- e.Type:
			default:
			}
		}
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(60 * time.Millisecond) // dejar que arranque a mandar chunks
	if err := client.Cancel(ctx, taskID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	select {
	case typ := <-terminal:
		if typ != EventCancelled {
			t.Fatalf("evento terminal = %q, want cancelled", typ)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nunca llegó el evento cancelled -- task.cancel no cortó la task viva")
	}
}

// TestSubscribe_Reconnect_ReplaysFromZero es el DoD de Fase 4: cerrar
// el cliente a mitad de una task y reabrirlo (un Subscribe nuevo,
// from=0) tiene que reenganchar con el historial completo, no perderlo.
func TestSubscribe_Reconnect_ReplaysFromZero(t *testing.T) {
	sock, client1 := startTestServer(t, mock.Config{Response: "contenido para reenganchar", ChunkDelay: 30 * time.Millisecond})
	// no defer Close en client1 -- lo cerramos a propósito a mitad de camino

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	taskID, err := client1.Submit(ctx, "hola", "m")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	firstChunk := make(chan struct{}, 1)
	if err := client1.Subscribe(ctx, taskID, 0, func(e Event) {
		if e.Type == EventChunk {
			select {
			case firstChunk <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatalf("Subscribe (client1): %v", err)
	}

	select {
	case <-firstChunk:
	case <-time.After(2 * time.Second):
		t.Fatal("nunca llegó el primer chunk")
	}
	client1.Close() // "cerrar la TUI a mitad de una task"

	// Dar tiempo a que la task siga corriendo y termine en el daemon
	// (el Cancel de contexto es del cliente que se cerró, no de la
	// task -- Server.run corre con el ctx del daemon, no el de la
	// conexión).
	time.Sleep(300 * time.Millisecond)

	client2, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial (reconexión): %v", err)
	}
	defer client2.Close()

	var mu sync.Mutex
	var replayed []Event
	done := make(chan struct{}, 1)
	if err := client2.Subscribe(ctx, taskID, 0, func(e Event) {
		mu.Lock()
		replayed = append(replayed, e)
		mu.Unlock()
		if e.Type != EventChunk {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatalf("Subscribe (client2, reconexión): %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("la reconexión nunca recibió el evento terminal")
	}

	mu.Lock()
	defer mu.Unlock()
	var content string
	for _, e := range replayed {
		if e.Type == EventChunk {
			content += e.Delta
		}
	}
	if content != "contenido para reenganchar" {
		t.Fatalf("reenganche no trajo el historial completo, got %q", content)
	}
}
