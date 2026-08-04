package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

// TestGetMessageDecodesDeliveryAndReadFields pins the fields the web API
// returns beyond the basic message shape — per-recipient delivery state,
// read state, and add-to batch IDs — so JSON output stays faithful to the
// API response.
func TestGetMessageDecodesDeliveryAndReadFields(t *testing.T) {
	const body = `{
		"version": 1, "has_pid": true, "pid": 41, "from": "@alice@example.com",
		"to": ["@bob@example.com"],
		"to_delivery": [
			{"addr": "@bob@example.com", "time_delivered": "2026-08-04T06:00:00Z", "response_code": 200},
			{"addr": "@carol@example.com", "time_delivered": null, "response_code": null}
		],
		"add_to": [
			{"batch_id": 7, "add_to_from": "@alice@example.com", "to": ["@carol@example.com"],
			 "to_delivery": [{"addr": "@carol@example.com", "time_delivered": null, "response_code": null}],
			 "time": 1754280000.5}
		],
		"time": 1754280000.0, "topic": "", "type": "text/markdown", "size": 12,
		"short_text": "hello world", "read": true, "time_read": 1754283600.25,
		"attachments": [{"filename": "claude-session.json", "size": 2048}]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/42" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	msg, err := client.GetMessage("42")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}

	if len(msg.ToDelivery) != 2 {
		t.Fatalf("ToDelivery length: want 2, got %d", len(msg.ToDelivery))
	}
	if msg.ToDelivery[0].TimeDelivered == nil || *msg.ToDelivery[0].TimeDelivered != "2026-08-04T06:00:00Z" {
		t.Fatalf("ToDelivery[0].TimeDelivered: got %v", msg.ToDelivery[0].TimeDelivered)
	}
	if msg.ToDelivery[0].ResponseCode == nil || *msg.ToDelivery[0].ResponseCode != 200 {
		t.Fatalf("ToDelivery[0].ResponseCode: got %v", msg.ToDelivery[0].ResponseCode)
	}
	if msg.ToDelivery[1].TimeDelivered != nil || msg.ToDelivery[1].ResponseCode != nil {
		t.Fatal("ToDelivery[1] should have null delivery state")
	}
	if len(msg.AddTo) != 1 || msg.AddTo[0].BatchID != 7 {
		t.Fatalf("AddTo batch_id: want 7, got %+v", msg.AddTo)
	}
	if len(msg.AddTo[0].ToDelivery) != 1 {
		t.Fatalf("AddTo[0].ToDelivery length: want 1, got %d", len(msg.AddTo[0].ToDelivery))
	}
	if !msg.Read || msg.TimeRead == nil || *msg.TimeRead != 1754283600.25 {
		t.Fatalf("read state: read=%v time_read=%v", msg.Read, msg.TimeRead)
	}
}

// TestUploadAttachmentReturnsServerResponse pins the upload response shape.
func TestUploadAttachmentReturnsServerResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/42/attach" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Fatalf("reading multipart file field: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"filename":"notes.txt","size":11}`))
	}))
	defer srv.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "notes-*.txt")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := tmp.WriteString("hello world"); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	tmp.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	result, err := client.UploadAttachment("42", tmp.Name())
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if result.Filename != "notes.txt" || result.Size != 11 {
		t.Fatalf("upload response: got %+v", result)
	}
}
