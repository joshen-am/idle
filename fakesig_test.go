package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Servidor de prueba que imita al de PeerJS (registro de ids y relevo de mensajes), con una
// demora de ida configurable para parecerse a internet.
func fakeSignal(delay time.Duration) (url string, stop func()) {
	var mu sync.Mutex
	peers := map[string]*websocket.Conn{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := context.Background()
		id := r.URL.Query().Get("id")
		mu.Lock()
		_, taken := peers[id]
		if !taken {
			peers[id] = c
		}
		mu.Unlock()
		if taken {
			c.Write(ctx, websocket.MessageText, []byte(`{"type":"ID-TAKEN"}`))
			c.Close(websocket.StatusNormalClosure, "")
			return
		}
		c.Write(ctx, websocket.MessageText, []byte(`{"type":"OPEN"}`))
		defer func() {
			mu.Lock()
			delete(peers, id)
			mu.Unlock()
		}()
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m sigMsg
			if json.Unmarshal(b, &m) != nil || m.Type != "CANDIDATE" {
				continue
			}
			m.Src = id
			out, _ := json.Marshal(m)
			mu.Lock()
			dst := peers[m.Dst]
			mu.Unlock()
			if dst != nil {
				time.AfterFunc(delay, func() { dst.Write(ctx, websocket.MessageText, out) })
			}
		}
	}))
	return "ws" + strings.TrimPrefix(srv.URL, "http"), srv.Close
}
