package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Client es un cliente IPC genérico -- lo usa tanto el "cliente tonto"
// (cmd/arg0s/daemon.go) como, más adelante, la TUI. No tiene lógica de
// negocio (P1): solo manda Request y despacha Response/Event.
type Client struct {
	conn   net.Conn
	nextID int64

	pendingM sync.Mutex
	pending  map[int64]chan Response

	eventsM sync.Mutex
	events  map[string][]func(Event) // task_id -> callbacks activos

	closed chan struct{}
}

func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial %s: %w", socketPath, err)
	}
	c := &Client{
		conn: conn, pending: map[int64]chan Response{},
		events: map[string][]func(Event){}, closed: make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) readLoop() {
	defer close(c.closed)
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Kind {
		case "response":
			if msg.Response == nil {
				continue
			}
			c.pendingM.Lock()
			ch, ok := c.pending[msg.Response.ID]
			c.pendingM.Unlock()
			if ok {
				ch <- *msg.Response
			}
		case "event":
			if msg.Event == nil {
				continue
			}
			c.eventsM.Lock()
			cbs := append([]func(Event){}, c.events[msg.Event.TaskID]...)
			c.eventsM.Unlock()
			for _, cb := range cbs {
				cb(*msg.Event)
			}
		}
	}
}

func (c *Client) call(ctx context.Context, method string, params any) (Response, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan Response, 1)
	c.pendingM.Lock()
	c.pending[id] = ch
	c.pendingM.Unlock()
	defer func() {
		c.pendingM.Lock()
		delete(c.pending, id)
		c.pendingM.Unlock()
	}()

	raw, err := json.Marshal(params)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(Request{ID: id, Method: method, Params: raw})
	if err != nil {
		return Response{}, err
	}
	body = append(body, '\n')
	if _, err := c.conn.Write(body); err != nil {
		return Response{}, fmt.Errorf("ipc: write: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != "" {
			return Response{}, fmt.Errorf("ipc: %s: %s", method, resp.Error)
		}
		return resp, nil
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-c.closed:
		return Response{}, fmt.Errorf("ipc: %s: conexión cerrada antes de responder", method)
	}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, MethodDaemonPing, struct{}{})
	return err
}

func (c *Client) Submit(ctx context.Context, prompt, model string) (string, error) {
	resp, err := c.call(ctx, MethodTaskSubmit, SubmitParams{Prompt: prompt, Model: model})
	if err != nil {
		return "", err
	}
	var result SubmitResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", err
	}
	return result.TaskID, nil
}

func (c *Client) Cancel(ctx context.Context, taskID string) error {
	_, err := c.call(ctx, MethodTaskCancel, CancelParams{TaskID: taskID})
	return err
}

// Subscribe registra onEvent para taskID y manda events.subscribe.
// onEvent se llama en la goroutine de readLoop -- que no bloquee mucho.
func (c *Client) Subscribe(ctx context.Context, taskID string, from int, onEvent func(Event)) error {
	c.eventsM.Lock()
	c.events[taskID] = append(c.events[taskID], onEvent)
	c.eventsM.Unlock()

	_, err := c.call(ctx, MethodEventsSubscribe, SubscribeParams{TaskID: taskID, From: from})
	return err
}
