// Package lsp es un cliente JSON-RPC genérico sobre stdio para
// hablarle a CUALQUIER servidor LSP (sección 13: "Escribir un
// extractor AST→SQLite con go-tree-sitter implica CGO... los
// servidores LSP ya resuelven símbolos, definiciones y referencias").
// Este paquete no sabe nada de gopls en particular -- eso vive en
// internal/codegraph/index.go, que es quien arma los params concretos
// de cada método (documentSymbol, references, etc).
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client es un proceso LSP vivo (gopls, pyright-langserver, ...)
// hablado por JSON-RPC 2.0 con framing Content-Length (el transporte
// estándar de LSP, RFC de Microsoft).
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writeM sync.Mutex

	nextID int64

	pendingM sync.Mutex
	pending  map[int64]chan rpcMessage

	closed chan struct{}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("lsp: %s (code %d)", e.Message, e.Code) }

// Start lanza el servidor LSP como subproceso y arranca el loop de
// lectura. dir es el working directory del proceso (la raíz del
// proyecto a indexar).
func Start(command string, args []string, dir string) (*Client, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %s: %w", command, err)
	}

	c := &Client{
		cmd: cmd, stdin: stdin,
		pending: map[int64]chan rpcMessage{},
		closed:  make(chan struct{}),
	}
	go c.readLoop(bufio.NewReader(stdout))
	return c, nil
}

// Initialize hace el handshake mínimo de LSP: initialize → initialized.
// rootURI es "file:///abs/path/al/proyecto".
func (c *Client) Initialize(ctx context.Context, rootURI string) error {
	params := map[string]any{
		"processId": nil,
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"references":     map[string]any{},
				"definition":     map[string]any{},
			},
		},
	}
	if _, err := c.Call(ctx, "initialize", params); err != nil {
		return fmt.Errorf("lsp: initialize: %w", err)
	}
	return c.Notify("initialized", map[string]any{})
}

// Call manda un request y bloquea hasta la respuesta (o ctx.Done()).
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan rpcMessage, 1)

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
		return nil, fmt.Errorf("lsp: marshal params for %s: %w", method, err)
	}
	msg := rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: raw}
	// write() bloquea en c.stdin.Write -- si el proceso LSP dejó de leer
	// (colgado o muerto), un Write directo ignoraría ctx para siempre.
	// Se manda en su propia goroutine para que ctx.Done() pueda cortar
	// la espera igual; la goroutine puede quedar escribiendo un rato
	// más si el pipe nunca drena, pero Call() ya no cuelga con ella.
	writeErr := make(chan error, 1)
	go func() { writeErr <- c.write(msg) }()

	select {
	case err := <-writeErr:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("lsp: %s: cliente cerrado antes de responder", method)
	}
}

// Notify manda una notificación (sin id, sin respuesta esperada).
func (c *Client) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("lsp: marshal params for %s: %w", method, err)
	}
	return c.write(rpcMessage{JSONRPC: "2.0", Method: method, Params: raw})
}

func (c *Client) write(msg rpcMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("lsp: marshal message: %w", err)
	}
	c.writeM.Lock()
	defer c.writeM.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("lsp: write header: %w", err)
	}
	_, err = c.stdin.Write(body)
	return err
}

// readLoop parsea el framing Content-Length y despacha cada mensaje:
// una respuesta a un Call pendiente va a su canal; todo lo demás
// (notificaciones del server, requests del server a nosotros como
// workspace/configuration) se ignora -- este cliente es solo para
// indexar, no implementa el lado "editor" del protocolo.
func (c *Client) readLoop(r *bufio.Reader) {
	defer close(c.closed)
	for {
		length, err := readContentLength(r)
		if err != nil {
			return
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(r, body); err != nil {
			return
		}

		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		if msg.ID == nil || msg.Method != "" {
			continue // notificación, o request del server -- no lo atendemos
		}

		c.pendingM.Lock()
		ch, ok := c.pending[*msg.ID]
		c.pendingM.Unlock()
		if ok {
			ch <- msg
		}
	}
}

func readContentLength(r *bufio.Reader) (int, error) {
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		if line == "\r\n" || line == "\n" {
			break // línea vacía: fin de headers
		}
		fmt.Sscanf(line, "Content-Length: %d", &length)
	}
	if length == 0 {
		return 0, fmt.Errorf("lsp: header sin Content-Length")
	}
	return length, nil
}

// Close apaga el servidor prolijamente (shutdown/exit) y espera al
// proceso. Nunca falla en silencio si el proceso ya murió -- se
// ignora el error de Wait en ese caso, no es un fallo del cliente.
func (c *Client) Close(ctx context.Context) error {
	_, _ = c.Call(ctx, "shutdown", nil)
	_ = c.Notify("exit", nil)
	_ = c.stdin.Close()
	_ = c.cmd.Wait()
	return nil
}
