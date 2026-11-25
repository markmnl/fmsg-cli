package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Event type discriminators pushed over GET /fmsg/ws. Unknown types are
// passed through untouched so new server events reach callers unchanged.
const (
	EventNewMsg          = "new_msg"
	EventDelivered       = "delivered"
	EventRecipientsAdded = "recipients_added"
	EventEdit            = "edit"
)

// WatchEvent is one frame from the WebSocket: a type discriminator and the
// event-specific body, kept raw so it can be re-emitted byte-for-byte.
type WatchEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Item decodes Data as a message list item (the shape of every event the
// server currently sends).
func (e WatchEvent) Item() (*MessageListItem, error) {
	var item MessageListItem
	if err := json.Unmarshal(e.Data, &item); err != nil {
		return nil, fmt.Errorf("decoding %s event: %w", e.Type, err)
	}
	return &item, nil
}

// ErrStopWatch may be returned by a WatchHandler to end Watch cleanly.
var ErrStopWatch = errors.New("stop watch")

// WatchHandler receives each event. Returning ErrStopWatch ends the watch
// with a nil error; any other error ends it with that error.
type WatchHandler func(WatchEvent) error

// WatchOptions tunes Watch.
type WatchOptions struct {
	// OnConnect is called after every successful handshake (including
	// reconnects), before any event from that connection is delivered.
	OnConnect func()
	// Reconnect re-dials with backoff when the connection drops or a transient
	// dial failure (network error or 5xx handshake response) occurs. Auth errors
	// and 4xx handshake failures (other than a single 401 token refresh retry)
	// fail fast.
	Reconnect bool
	// Dialer overrides the WebSocket dialer (tests).
	Dialer *websocket.Dialer
}

// readTimeout bounds silence on the socket. The server pings every 45s, so a
// healthy connection always produces a frame well inside this window.
const (
	readTimeout          = 90 * time.Second
	maxWatchMessageBytes = 4 << 20
)

// Backoff and health parameters (vars so unit tests can shorten them).
var (
	watchInitialBackoff  = time.Second
	watchMaxBackoff      = 30 * time.Second
	watchHealthyDuration = 30 * time.Second
)

func nextWatchBackoff(cur time.Duration) time.Duration {
	cur *= 2
	if cur > watchMaxBackoff {
		return watchMaxBackoff
	}
	return cur
}

func waitBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Watch connects to GET /fmsg/ws and calls fn for every pushed event until
// ctx is cancelled, fn returns an error, or (with Reconnect off) the
// connection drops.
func (c *Client) Watch(ctx context.Context, opts WatchOptions, fn WatchHandler) error {
	backoff := watchInitialBackoff
	for {
		conn, err := c.dialWatch(ctx, opts.Dialer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !opts.Reconnect || !isTransientDialError(err) {
				return err
			}
			if err := waitBackoff(ctx, backoff); err != nil {
				return err
			}
			backoff = nextWatchBackoff(backoff)
			continue
		}

		connectedAt := time.Now()
		if opts.OnConnect != nil {
			opts.OnConnect()
		}
		eventsRead, err := readEvents(ctx, conn, fn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrStopWatch) {
			return nil
		}
		var readErr *watchReadError
		if !opts.Reconnect || !errors.As(err, &readErr) {
			return err
		}

		// Only reset backoff when the connection proved healthy (delivered at
		// least one event or remained open for watchHealthyDuration).
		if eventsRead > 0 || time.Since(connectedAt) >= watchHealthyDuration {
			backoff = watchInitialBackoff
		}
		if err := waitBackoff(ctx, backoff); err != nil {
			return err
		}
		backoff = nextWatchBackoff(backoff)
	}
}

// watchReadError marks a connection-level failure (as opposed to a handler
// error) so Watch knows a reconnect is appropriate.
type watchReadError struct{ err error }

func (e *watchReadError) Error() string { return "websocket: " + e.err.Error() }
func (e *watchReadError) Unwrap() error { return e.err }

// watchDialNetworkError marks a transport/network failure during WebSocket dial
// so Watch can distinguish transient dial failures from auth errors.
type watchDialNetworkError struct{ err error }

func (e *watchDialNetworkError) Error() string { return "network error: " + e.err.Error() }
func (e *watchDialNetworkError) Unwrap() error { return e.err }

func isTransientDialError(err error) bool {
	var netErr *watchDialNetworkError
	if errors.As(err, &netErr) {
		return true
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.StatusCode >= 500 && ae.StatusCode < 600
	}
	return false
}

// WatchURL returns the WebSocket endpoint derived from BaseURL.
func (c *Client) WatchURL() string {
	u := c.BaseURL + "/fmsg/ws"
	switch {
	case strings.HasPrefix(u, "https://"):
		return "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		return "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

// dialWatch performs the handshake with the bearer token in the
// Authorization header (never the query string, which lands in logs). A 401
// is retried once with a force-refreshed token, mirroring Client.do.
func (c *Client) dialWatch(ctx context.Context, dialer *websocket.Dialer) (*websocket.Conn, error) {
	if c.Auth == nil {
		return nil, fmt.Errorf("missing token provider")
	}
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}

	dialWithToken := func(forceRefresh bool) (*websocket.Conn, *http.Response, error, bool) {
		token, err := c.Auth.AccessToken(ctx, forceRefresh)
		if err != nil {
			return nil, nil, err, true
		}
		h := http.Header{}
		h.Set("Authorization", "Bearer "+token)
		conn, resp, err := dialer.DialContext(ctx, c.WatchURL(), h)
		return conn, resp, err, false
	}

	conn, resp, err, authErr := dialWithToken(false)
	if err != nil && !authErr && resp != nil && resp.StatusCode == http.StatusUnauthorized {
		drainAndClose(resp.Body)
		conn, resp, err, authErr = dialWithToken(true)
	}
	if err != nil {
		if authErr {
			return nil, err
		}
		if resp != nil {
			return nil, &apiError{
				StatusCode: resp.StatusCode,
				Body:       readErrorBody(resp.Body, resp.StatusCode),
			}
		}
		return nil, &watchDialNetworkError{err: err}
	}
	return conn, nil
}

func readEvents(ctx context.Context, conn *websocket.Conn, fn WatchHandler) (int, error) {
	// Close the socket when ctx ends so a blocked ReadMessage returns.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	conn.SetReadLimit(maxWatchMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(readTimeout)) })
	// gorilla's default ping handler answers with a pong; it also needs the
	// deadline pushed, since a ping is proof the server is alive.
	conn.SetPingHandler(func(data string) error {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		err := conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})

	eventsRead := 0
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return eventsRead, &watchReadError{err}
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		var ev WatchEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			return eventsRead, &watchReadError{fmt.Errorf("decoding event: %w", err)}
		}
		if err := fn(ev); err != nil {
			return eventsRead, err
		}
		eventsRead++
	}
}
