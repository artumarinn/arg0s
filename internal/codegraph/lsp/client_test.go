package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// newTestClient conecta un *Client a un servidor LSP falso vía dos
// io.Pipe -- sin gopls real, sin proceso, sin red. El cliente contra
// gopls de verdad va en index_test.go detrás de //go:build integration
// (requiere el binario instalado); esto prueba el framing/dispatch de
// JSON-RPC, que es responsabilidad de ESTE paquete.
func newTestClient(t *testing.T, handle func(req rpcMessage) rpcMessage) *Client {
	t.Helper()
	serverReadsFrom, clientWritesTo := io.Pipe()
	clientReadsFrom, serverWritesTo := io.Pipe()

	c := &Client{
		stdin:   clientWritesTo,
		pending: map[int64]chan rpcMessage{},
		closed:  make(chan struct{}),
	}
	go c.readLoop(bufio.NewReader(clientReadsFrom))

	go func() {
		r := bufio.NewReader(serverReadsFrom)
		for {
			length, err := readContentLength(r)
			if err != nil {
				return
			}
			body := make([]byte, length)
			if _, err := io.ReadFull(r, body); err != nil {
				return
			}
			var req rpcMessage
			if err := json.Unmarshal(body, &req); err != nil {
				continue
			}
			if req.ID == nil {
				continue // notificación del cliente -- el fake server no responde nada
			}
			resp := handle(req)
			resp.JSONRPC, resp.ID = "2.0", req.ID
			respBody, _ := json.Marshal(resp)
			serverWritesTo.Write([]byte("Content-Length: " + itoa(len(respBody)) + "\r\n\r\n"))
			serverWritesTo.Write(respBody)
		}
	}()

	t.Cleanup(func() { clientWritesTo.Close() })
	return c
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestCall_RoundTripsRequestAndResponse(t *testing.T) {
	c := newTestClient(t, func(req rpcMessage) rpcMessage {
		if req.Method != "textDocument/documentSymbol" {
			t.Errorf("unexpected method %q", req.Method)
		}
		return rpcMessage{Result: json.RawMessage(`[{"name":"Foo","kind":12}]`)}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := c.Call(ctx, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": "file:///x.go"}})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var got []struct {
		Name string `json:"name"`
		Kind int    `json:"kind"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Foo" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCall_ServerError_PropagatesAsError(t *testing.T) {
	c := newTestClient(t, func(req rpcMessage) rpcMessage {
		return rpcMessage{Error: &rpcError{Code: -32601, Message: "method not found"}}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, "bogus/method", nil); err == nil {
		t.Fatal("esperaba error del server, no hubo")
	}
}

func TestCall_ContextCancelled_ReturnsBeforeResponse(t *testing.T) {
	c := &Client{pending: map[int64]chan rpcMessage{}, closed: make(chan struct{})}
	_, clientWritesTo := io.Pipe()
	clientReadsFrom, _ := io.Pipe() // el server nunca escribe -- simula un método que cuelga
	c.stdin = clientWritesTo
	go c.readLoop(bufio.NewReader(clientReadsFrom))
	t.Cleanup(func() { clientWritesTo.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Call(ctx, "never/responds", nil); err == nil {
		t.Fatal("esperaba error por ctx cancelado, no hubo")
	}
}
