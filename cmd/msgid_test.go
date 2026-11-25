package cmd

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	id, err := resolveMessageID(context.Background(), client, "42")
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
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	id, err := resolveMessageID(context.Background(), client, "-1")
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
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	id, err := resolveMessageID(context.Background(), client, "-2")
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
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	id, err := resolveMessageID(context.Background(), client, "-3")
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
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	_, err := resolveMessageID(context.Background(), client, "-4")
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestResolveMessageID_Zero(t *testing.T) {
	client := api.New("http://unused", api.StaticTokenProvider("token"))
	_, err := resolveMessageID(context.Background(), client, "0")
	if err == nil {
		t.Fatal("expected error for ID=0")
	}
}

func TestResolveMessageID_MinInt64DoesNotPanic(t *testing.T) {
	client := api.New("http://unused", api.StaticTokenProvider("token"))
	_, err := resolveMessageID(context.Background(), client, strconv.FormatInt(math.MinInt64, 10))
	if err == nil {
		t.Fatal("expected error for math.MinInt64")
	}
}

func TestResolveMessageID_UsesSmallAdaptivePageSize(t *testing.T) {
	var limits []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		limits = append(limits, limit)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]api.MessageListItem{{ID: 77}})
	}))
	defer srv.Close()

	client := api.New(srv.URL, api.StaticTokenProvider("token"))
	id, err := resolveMessageID(context.Background(), client, "-1")
	if err != nil {
		t.Fatalf("resolveMessageID: %v", err)
	}
	if id != 77 {
		t.Fatalf("id = %d, want 77", id)
	}
	if len(limits) != 1 || limits[0] != 10 {
		t.Fatalf("limits = %v, want [10]", limits)
	}
}

func TestResolveMessageID_Invalid(t *testing.T) {
	client := api.New("http://unused", api.StaticTokenProvider("token"))
	_, err := resolveMessageID(context.Background(), client, "abc")
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

// TestResolveMessageID_SkipsEditMessages pins that negative indices count the
// messages "list" shows: edit messages (FMSG-008) are passed over.
func TestResolveMessageID_SkipsEditMessages(t *testing.T) {
	pid := int64(50)
	items := []api.MessageListItem{
		{ID: 100, Edit: true, PID: &pid},
		{ID: 90},
		{ID: 80, Edit: true, PID: &pid},
		{ID: 50},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page := items
		if offset >= len(page) {
			page = nil
		} else {
			page = page[offset:]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()
	client := api.New(srv.URL, api.StaticTokenProvider("token"))

	for arg, want := range map[string]int64{"-1": 90, "-2": 50} {
		id, err := resolveMessageID(context.Background(), client, arg)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", arg, err)
		}
		if id != want {
			t.Fatalf("%s: want %d, got %d", arg, want, id)
		}
	}
	if _, err := resolveMessageID(context.Background(), client, "-3"); err == nil {
		t.Fatal("-3: want an error, only two messages are visible")
	}
}

func TestListLineMarksEditsAndEditedMessages(t *testing.T) {
	pid := int64(7)
	size := 5
	cases := []struct {
		msg  api.MessageListItem
		want string
	}{
		{api.MessageListItem{ID: 7, From: "@a@example.com"}, `ID: 7  From: @a@example.com  To: []`},
		{api.MessageListItem{ID: 7, From: "@a@example.com", Edited: &api.Edited{ID: 9, Time: 1, Size: &size}}, `ID: 7  From: @a@example.com  To: []  (edited)`},
		{api.MessageListItem{ID: 9, From: "@a@example.com", Edit: true, PID: &pid}, `ID: 9  From: @a@example.com  To: []  (edit of 7)`},
	}
	for _, c := range cases {
		if got := listLine(c.msg, "[]"); got != c.want {
			t.Errorf("listLine = %q, want %q", got, c.want)
		}
	}
}

func TestReadCredentialFromEOFWithoutNewline(t *testing.T) {
	got, err := readCredentialFrom(strings.NewReader("fmsgk_no_newline"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fmsgk_no_newline" {
		t.Fatalf("got %q, want %q", got, "fmsgk_no_newline")
	}

	got, err = readCredentialFrom(strings.NewReader("fmsgk_with_newline\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(got) != "fmsgk_with_newline" {
		t.Fatalf("got %q", got)
	}

	if _, err := readCredentialFrom(strings.NewReader("")); err == nil {
		t.Fatal("expected error on empty EOF")
	}
}

func TestReadMessageContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "body.txt")
	if err := os.WriteFile(file, []byte("from file"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := readMessageContent(file)
	if err != nil || string(got) != "from file" {
		t.Fatalf("readMessageContent(file) = %q, %v", string(got), err)
	}

	got, err = readMessageContent("literal text message")
	if err != nil || string(got) != "literal text message" {
		t.Fatalf("readMessageContent(literal) = %q, %v", string(got), err)
	}

	// Directory falls back to literal text.
	got, err = readMessageContent(dir)
	if err != nil || string(got) != dir {
		t.Fatalf("readMessageContent(dir) = %q, %v", string(got), err)
	}

	// Existing unreadable regular file must return an error instead of falling back.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		unreadable := filepath.Join(dir, "secret.txt")
		if err := os.WriteFile(unreadable, []byte("secret"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(unreadable, 0000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0600) })

		if _, err := readMessageContent(unreadable); err == nil {
			t.Fatal("expected explicit error for unreadable existing regular file")
		}
	}
}
