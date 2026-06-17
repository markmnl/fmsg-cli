package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRefreshesTokenAndRetriesReplayableRequestOn401(t *testing.T) {
	provider := &sequenceTokenProvider{}
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if r.URL.Path != "/fmsg" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		if string(body) != `{"hello":"world"}` {
			t.Fatalf("request body was not replayed, got %q", string(body))
		}
		switch attempt {
		case 1:
			if got, want := r.Header.Get("Authorization"), "Bearer stale-token"; got != want {
				t.Fatalf("first Authorization: want %q, got %q", want, got)
			}
			http.Error(w, "expired", http.StatusUnauthorized)
		case 2:
			if got, want := r.Header.Get("Authorization"), "Bearer fresh-token"; got != want {
				t.Fatalf("retry Authorization: want %q, got %q", want, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":123}`))
		default:
			t.Fatalf("unexpected extra request attempt %d", attempt)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, provider)
	resp, err := client.CreateMessage([]byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if resp.ID != 123 {
		t.Fatalf("response ID: want 123, got %d", resp.ID)
	}
	if !provider.sawForceRefresh {
		t.Fatal("expected force refresh after 401")
	}
}

type sequenceTokenProvider struct {
	sawForceRefresh bool
}

func (p *sequenceTokenProvider) AccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	if forceRefresh {
		p.sawForceRefresh = true
		return "fresh-token", nil
	}
	return "stale-token", nil
}
