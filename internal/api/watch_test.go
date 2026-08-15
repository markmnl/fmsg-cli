package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// refreshingProvider hands out "stale" first, then "fresh" once refreshed.
type refreshingProvider struct{ refreshed atomic.Bool }

func (p *refreshingProvider) AccessToken(ctx context.Context, force bool) (string, error) {
	if force {
		p.refreshed.Store(true)
	}
	if p.refreshed.Load() {
		return "fresh", nil
	}
	return "stale", nil
}

// wsServer serves /fmsg/ws, accepting only Bearer "fresh", and runs serve on
// each upgraded connection.
func wsServer(t *testing.T, serve func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	up := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/ws" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("access_token") != "" {
			t.Errorf("token must travel in the header, not the query string")
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		serve(conn)
	}))
}

func TestWatchURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8000":    "ws://127.0.0.1:8000/fmsg/ws",
		"https://api.example.com/": "wss://api.example.com/fmsg/ws",
	} {
		if got := New(in, StaticTokenProvider("x")).WatchURL(); got != want {
			t.Errorf("WatchURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWatchDeliversEventsRefreshesTokenAndStops(t *testing.T) {
	srv := wsServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":7,"from":"@a@x","to":["@b@y"]}}`))
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"delivered","data":{"id":8}}`))
		// Keep the connection open until the client goes away.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	prov := &refreshingProvider{}
	c := New(srv.URL, prov)
	var got []WatchEvent
	connects := 0
	err := c.Watch(context.Background(), WatchOptions{OnConnect: func() { connects++ }}, func(ev WatchEvent) error {
		got = append(got, ev)
		if len(got) == 2 {
			return ErrStopWatch
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if !prov.refreshed.Load() {
		t.Errorf("expected a forced token refresh after 401")
	}
	if connects != 1 {
		t.Errorf("OnConnect called %d times, want 1", connects)
	}
	if len(got) != 2 || got[0].Type != "new_msg" || got[1].Type != "delivered" {
		t.Fatalf("events = %+v", got)
	}
	item, err := got[0].Item()
	if err != nil || item.ID != 7 || item.From != "@a@x" {
		t.Errorf("Item() = %+v, %v", item, err)
	}
}

func TestWatchHandlerErrorPropagates(t *testing.T) {
	srv := wsServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":1}}`))
		conn.ReadMessage()
	})
	defer srv.Close()
	boom := errors.New("boom")
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(context.Background(), WatchOptions{}, func(WatchEvent) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestWatchReconnectsAndReportsReady(t *testing.T) {
	var conns atomic.Int32
	srv := wsServer(t, func(conn *websocket.Conn) {
		n := conns.Add(1)
		if n == 1 {
			return // drop the first connection immediately
		}
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":2}}`))
		conn.ReadMessage()
	})
	defer srv.Close()

	ready := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(ctx, WatchOptions{
		Reconnect: true,
		OnConnect: func() { ready++ },
	}, func(ev WatchEvent) error { return ErrStopWatch })
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ready != 2 || conns.Load() != 2 {
		t.Errorf("ready=%d conns=%d, want 2/2", ready, conns.Load())
	}
}

func TestWatchWithoutReconnectReturnsDropError(t *testing.T) {
	srv := wsServer(t, func(conn *websocket.Conn) {})
	defer srv.Close()
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(context.Background(), WatchOptions{}, func(WatchEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "websocket") {
		t.Fatalf("err = %v, want websocket read error", err)
	}
}

func TestWatchContextCancel(t *testing.T) {
	srv := wsServer(t, func(conn *websocket.Conn) { conn.ReadMessage() })
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(srv.URL, StaticTokenProvider("fresh")).Watch(ctx, WatchOptions{Reconnect: true}, func(WatchEvent) error { return nil })
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return after cancel")
	}
}

func TestWatchHandshakeFailureIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(context.Background(), WatchOptions{Reconnect: true}, func(WatchEvent) error { return nil })
	var ae *apiError
	if !errors.As(err, &ae) || ae.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %v, want apiError 403", err)
	}
}
