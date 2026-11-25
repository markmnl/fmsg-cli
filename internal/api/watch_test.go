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
	if ae.Body != "forbidden" {
		t.Fatalf("body = %q, want %q", ae.Body, "forbidden")
	}
}

func withShortWatchBackoff(t *testing.T, init, max, healthy time.Duration) {
	t.Helper()
	origInit, origMax, origHealthy := watchInitialBackoff, watchMaxBackoff, watchHealthyDuration
	watchInitialBackoff = init
	watchMaxBackoff = max
	watchHealthyDuration = healthy
	t.Cleanup(func() {
		watchInitialBackoff = origInit
		watchMaxBackoff = origMax
		watchHealthyDuration = origHealthy
	})
}

func TestWatchBackoffGrowsOnImmediateDropsAndResetsWhenHealthy(t *testing.T) {
	withShortWatchBackoff(t, 20*time.Millisecond, 80*time.Millisecond, 5*time.Second)

	var attempts atomic.Int32
	var timestamps []time.Time
	srv := wsServer(t, func(conn *websocket.Conn) {
		n := attempts.Add(1)
		switch n {
		case 1, 2, 3:
			return // immediate drop with 0 events: backoff should grow 20ms -> 40ms -> 80ms
		case 4:
			// Deliver an event (healthy), then drop: backoff should reset to 20ms.
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":10}}`))
			return
		default:
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":11}}`))
			_, _, _ = conn.ReadMessage()
		}
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := 0
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(ctx, WatchOptions{
		Reconnect: true,
		OnConnect: func() {
			timestamps = append(timestamps, time.Now())
		},
	}, func(ev WatchEvent) error {
		events++
		if events == 2 {
			return ErrStopWatch
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if len(timestamps) != 5 {
		t.Fatalf("connect count = %d, want 5", len(timestamps))
	}

	gap1 := timestamps[1].Sub(timestamps[0]) // ~20ms
	gap2 := timestamps[2].Sub(timestamps[1]) // ~40ms
	gap3 := timestamps[3].Sub(timestamps[2]) // ~80ms
	gap4 := timestamps[4].Sub(timestamps[3]) // reset to ~20ms after healthy conn #4

	if gap2 < 30*time.Millisecond || gap3 < 65*time.Millisecond {
		t.Fatalf("expected exponential backoff growth on unhealthy drops: gaps=%v, %v, %v", gap1, gap2, gap3)
	}
	if gap4 >= gap3 {
		t.Fatalf("expected backoff reset after healthy connection: gap3=%v gap4=%v", gap3, gap4)
	}
}

func TestWatchRetriesTransient5xxDialFailures(t *testing.T) {
	withShortWatchBackoff(t, 15*time.Millisecond, 60*time.Millisecond, 5*time.Second)

	var hits atomic.Int32
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= 2 {
			http.Error(w, "temporary overload", http.StatusServiceUnavailable)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"new_msg","data":{"id":99}}`))
		_, _, _ = conn.ReadMessage()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got WatchEvent
	err := New(srv.URL, StaticTokenProvider("fresh")).Watch(ctx, WatchOptions{Reconnect: true}, func(ev WatchEvent) error {
		got = ev
		return ErrStopWatch
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3 (2 transient 503s + 1 upgrade)", hits.Load())
	}
	if got.Type != "new_msg" {
		t.Fatalf("got event = %+v", got)
	}
}

type failingTokenProvider struct {
	err   error
	calls atomic.Int32
}

func (p *failingTokenProvider) AccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	p.calls.Add(1)
	return "", p.err
}

func TestWatchAuthErrorFailsFastWithoutNetworkWrapper(t *testing.T) {
	withShortWatchBackoff(t, 10*time.Millisecond, 40*time.Millisecond, 5*time.Second)

	authErr := errors.New("stored user JWT is expired")
	prov := &failingTokenProvider{err: authErr}

	err := New("http://127.0.0.1:1", prov).Watch(context.Background(), WatchOptions{Reconnect: true}, func(WatchEvent) error { return nil })
	if !errors.Is(err, authErr) {
		t.Fatalf("err = %v, want %v", err, authErr)
	}
	if strings.Contains(err.Error(), "network error") {
		t.Fatalf("auth error must not be wrapped as network error: %v", err)
	}
	if prov.calls.Load() != 1 {
		t.Fatalf("AccessToken called %d times, want 1 (fail fast)", prov.calls.Load())
	}
}

func TestWatchNextBackoffCapsAtMax(t *testing.T) {
	withShortWatchBackoff(t, time.Second, 30*time.Second, 30*time.Second)
	got := []time.Duration{}
	b := watchInitialBackoff
	for range 7 {
		got = append(got, b)
		b = nextWatchBackoff(b)
	}
	want := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
		30 * time.Second,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff[%d] = %v, want %v (full=%v)", i, got[i], want[i], got)
		}
	}
}
