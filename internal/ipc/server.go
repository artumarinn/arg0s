package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/google/uuid"
)

// Server es arg0sd -- sección 18.1: "ejecutar orquestación asíncrona
// dentro del event loop de una TUI produce UI congelada". El daemon
// corre las tasks; el cliente (dumb hoy, TUI en el próximo paso) solo
// manda comandos y recibe eventos.
type Server struct {
	exec     *execution.Executor
	fallback string

	mu    sync.Mutex
	tasks map[string]*task
}

func NewServer(exec *execution.Executor, fallback string) *Server {
	return &Server{exec: exec, fallback: fallback, tasks: map[string]*task{}}
}

// ListenAndServe escucha en socketPath (se borra un socket viejo
// huérfano si existe -- un daemon anterior que murió mal no debe
// impedir que el próximo arranque) y sirve conexiones hasta que ctx se
// cancela.
func (s *Server) ListenAndServe(ctx context.Context, socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ipc: limpiar socket viejo %s: %w", socketPath, err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("ipc: listen %s: %w", socketPath, err)
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("ipc: accept: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var writeMu sync.Mutex
	writeMsg := func(m Message) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		enc := json.NewEncoder(conn)
		return enc.Encode(m)
	}
	writeResp := func(resp Response) { _ = writeMsg(Message{Kind: "response", Response: &resp}) }

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeResp(Response{Error: "request inválido: " + err.Error()})
			continue
		}

		switch req.Method {
		case MethodDaemonPing:
			writeResp(Response{ID: req.ID, Result: json.RawMessage(`"pong"`)})

		case MethodTaskSubmit:
			var params SubmitParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResp(Response{ID: req.ID, Error: err.Error()})
				continue
			}
			taskID := s.submit(ctx, params)
			result, _ := json.Marshal(SubmitResult{TaskID: taskID})
			writeResp(Response{ID: req.ID, Result: result})

		case MethodTaskCancel:
			var params CancelParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResp(Response{ID: req.ID, Error: err.Error()})
				continue
			}
			if t, ok := s.get(params.TaskID); ok {
				t.Cancel()
				writeResp(Response{ID: req.ID, Result: json.RawMessage(`"ok"`)})
			} else {
				writeResp(Response{ID: req.ID, Error: "task no encontrada: " + params.TaskID})
			}

		case MethodEventsSubscribe:
			var params SubscribeParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResp(Response{ID: req.ID, Error: err.Error()})
				continue
			}
			t, ok := s.get(params.TaskID)
			if !ok {
				writeResp(Response{ID: req.ID, Error: "task no encontrada: " + params.TaskID})
				continue
			}
			writeResp(Response{ID: req.ID, Result: json.RawMessage(`"subscribed"`)})
			// En su propia goroutine: así el read loop de esta conexión
			// sigue atendiendo task.cancel mientras los eventos se
			// escriben -- full duplex sobre la misma conexión (sección
			// 18.4 DoD: ctrl+c cancela sin matar el cliente).
			go func() {
				_ = t.Subscribe(ctx, params.From, func(e Event) error {
					return writeMsg(Message{Kind: "event", Event: &e})
				})
			}()

		default:
			writeResp(Response{ID: req.ID, Error: "método desconocido: " + req.Method})
		}
	}
}

func (s *Server) get(taskID string) (*task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[taskID]
	return t, ok
}

// submit arranca la task en background y devuelve su ID de inmediato
// -- task.submit no espera a que termine (sección 18.2: el cliente se
// entera del progreso vía events.subscribe).
func (s *Server) submit(parentCtx context.Context, params SubmitParams) string {
	taskID := "tsk_" + uuid.NewString()
	ctx, cancel := context.WithCancel(parentCtx)
	t := newTask(cancel)

	s.mu.Lock()
	s.tasks[taskID] = t
	s.mu.Unlock()

	go s.run(ctx, taskID, t, params)
	return taskID
}

// run consume Executor.Stream y traduce cada core.Chunk a un Event,
// terminando SIEMPRE con un evento explícito (lección de streaming,
// tercera aparición -- ver protocol.go). El chequeo post-loop de
// ctx.Err() es el mismo patrón que internal/execution/stream.go:87-94
// -- acá aplica sobre run.Err, que YA lo hizo Executor.Stream
// internamente; lo que hace ESTE código es no perder esa distinción al
// traducir a Event (un run.Err == context.Canceled se manda como
// EventCancelled, cualquier otro error como EventFailed -- nunca un
// EventDone si run.Err != nil).
func (s *Server) run(ctx context.Context, taskID string, t *task, params SubmitParams) {
	req := core.Request{ModelID: params.Model, Prompt: params.Prompt, Role: core.RoleGenerator, Fallback: s.fallback}
	ch, run, err := s.exec.Stream(ctx, req)
	if err != nil {
		t.append(Event{TaskID: taskID, Type: EventFailed, Error: err.Error()})
		return
	}

	for c := range ch {
		if c.Delta != "" {
			t.append(Event{TaskID: taskID, Type: EventChunk, Delta: c.Delta})
		}
	}

	switch {
	case run.Err == nil:
		t.append(Event{TaskID: taskID, Type: EventDone})
	case errors.Is(run.Err, context.Canceled):
		t.append(Event{TaskID: taskID, Type: EventCancelled})
	default:
		t.append(Event{TaskID: taskID, Type: EventFailed, Error: run.Err.Error()})
	}
}

// Logf es un logger mínimo -- arg0sd corre sin terminal interactiva,
// así que cualquier error de accept/listen tiene que ir a algún lado
// que no sea perderse en silencio.
func Logf(format string, args ...any) { log.Printf(format, args...) }
