package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/markmnl/fmsg-cli/internal/api"
)

// newTestServer returns an httptest.Server that serves a fixed list of message
// IDs in descending order, matching the real API's behaviour.
func newTestServer(t *testing.T, ids []int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 20
		offset := 0
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				offset = n
			}
		}

		slice := ids
		if offset >= len(slice) {
			slice = nil
		} else {
			slice = slice[offset:]
		}
		if limit < len(slice) {
			slice = slice[:limit]
		}

		items := make([]api.MessageListItem, len(slice))
		for i, id := range slice {
			items[i] = api.MessageListItem{ID: id}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
}

func TestResolveMessageID_Positive(t *testing.T) {
	srv := newTestServer(t, []int64{100, 50, 10})
	defer srv.Close()
	client := api.New(srv.URL, "token")

	id, err := resolveMessageID(client, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("want 42, got %d", id)
	}
}

func TestResolveMessageID_NegativeOne(t *testing.T) {
	srv := newTestServer(t, []int64{100, 50, 10})
	defer srv.Close()
	client := api.New(srv.URL, "token")

	id, err := resolveMessageID(client, "-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 100 {
		t.Fatalf("want 100 (most recent), got %d", id)
	}
}

func TestResolveMessageID_NegativeTwo(t *testing.T) {
	srv := newTestServer(t, []int64{100, 50, 10})
	defer srv.Close()
	client := api.New(srv.URL, "token")

	id, err := resolveMessageID(client, "-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 50 {
		t.Fatalf("want 50 (second most recent), got %d", id)
	}
}

func TestResolveMessageID_NegativeThree(t *testing.T) {
	srv := newTestServer(t, []int64{100, 50, 10})
	defer srv.Close()
	client := api.New(srv.URL, "token")

	id, err := resolveMessageID(client, "-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 10 {
		t.Fatalf("want 10 (third most recent), got %d", id)
	}
}

func TestResolveMessageID_NegativeOutOfRange(t *testing.T) {
	srv := newTestServer(t, []int64{100, 50, 10})
	defer srv.Close()
	client := api.New(srv.URL, "token")

	_, err := resolveMessageID(client, "-4")
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestResolveMessageID_Zero(t *testing.T) {
	client := api.New("http://unused", "token")
	_, err := resolveMessageID(client, "0")
	if err == nil {
		t.Fatal("expected error for ID=0")
	}
}

func TestResolveMessageID_Invalid(t *testing.T) {
	client := api.New("http://unused", "token")
	_, err := resolveMessageID(client, "abc")
	if err == nil {
		t.Fatal("expected error for non-numeric input")
	}
}

func TestInjectDashDash_NegativeArg(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fmsg", "get", "-1"}
	injectDashDash()
	want := []string{"fmsg", "get", "--", "-1"}
	if len(os.Args) != len(want) {
		t.Fatalf("want %v, got %v", want, os.Args)
	}
	for i, a := range want {
		if os.Args[i] != a {
			t.Fatalf("arg[%d]: want %q, got %q", i, a, os.Args[i])
		}
	}
}

func TestInjectDashDash_AlreadyPresent(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fmsg", "get", "--", "-1"}
	injectDashDash()
	// Should not insert another --
	if len(os.Args) != 4 {
		t.Fatalf("expected 4 args, got %v", os.Args)
	}
}

func TestInjectDashDash_PositiveArgUnchanged(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fmsg", "get", "42"}
	injectDashDash()
	if len(os.Args) != 3 {
		t.Fatalf("expected 3 args unchanged, got %v", os.Args)
	}
}

func TestInjectDashDash_FlagBeforeNegative(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fmsg", "update", "--topic", "hello", "-1"}
	injectDashDash()
	want := []string{"fmsg", "update", "--topic", "hello", "--", "-1"}
	if len(os.Args) != len(want) {
		t.Fatalf("want %v, got %v", want, os.Args)
	}
	for i, a := range want {
		if os.Args[i] != a {
			t.Fatalf("arg[%d]: want %q, got %q", i, a, os.Args[i])
		}
	}
}
