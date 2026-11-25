package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	resp, err := client.CreateMessage(context.Background(), []byte(`{"hello":"world"}`))
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
	msg, err := client.GetMessage(context.Background(), "42")
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

// TestUploadAttachmentReturnsServerResponse pins the upload response shape and
// verifies regular files send an exact Content-Length.
func TestUploadAttachmentReturnsServerResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/42/attach" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.ContentLength <= 0 {
			t.Fatalf("expected positive Content-Length for regular file, got %d", r.ContentLength)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("reading multipart file field: %v", err)
		}
		defer f.Close()
		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("reading uploaded bytes: %v", err)
		}
		if string(got) != "hello world" {
			t.Fatalf("uploaded content = %q, want %q", string(got), "hello world")
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
	result, err := client.UploadAttachment(context.Background(), "42", tmp.Name())
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if result.Filename != "notes.txt" || result.Size != 11 {
		t.Fatalf("upload response: got %+v", result)
	}
}

func TestUploadAttachmentStreamingReplaysOn401(t *testing.T) {
	content := strings.Repeat("stream-payload-chunk-", 4096)
	provider := &sequenceTokenProvider{}
	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.ContentLength <= int64(len(content)) {
			t.Fatalf("attempt %d: expected exact multipart Content-Length > %d, got %d", attempts, len(content), r.ContentLength)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("attempt %d: FormFile: %v", attempts, err)
		}
		got, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatalf("attempt %d: reading file: %v", attempts, err)
		}
		if string(got) != content {
			t.Fatalf("attempt %d: body mismatch (len %d vs %d)", attempts, len(got), len(content))
		}
		if hdr.Filename != "report.bin" {
			t.Fatalf("attempt %d: filename = %q, want report.bin", attempts, hdr.Filename)
		}

		switch attempts {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer stale-token" {
				t.Fatalf("attempt 1 Authorization = %q", got)
			}
			http.Error(w, "expired", http.StatusUnauthorized)
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer fresh-token" {
				t.Fatalf("attempt 2 Authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"filename":"report.bin","size":86016}`))
		default:
			t.Fatalf("unexpected attempt %d", attempts)
		}
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "report.bin")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	client := New(srv.URL, provider)
	res, err := client.UploadAttachment(context.Background(), "42", path)
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if attempts != 2 || !provider.sawForceRefresh {
		t.Fatalf("attempts=%d sawForceRefresh=%v, want 2/true", attempts, provider.sawForceRefresh)
	}
	if res.Filename != "report.bin" {
		t.Fatalf("result = %+v", res)
	}
}

func TestAttachmentAndMessagePathEscaping(t *testing.T) {
	var rawPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPaths = append(rawPaths, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("file-bytes"))
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	outPath := filepath.Join(t.TempDir(), "out.txt")

	const filename = "my report #1 100%?.txt"
	if err := client.DownloadAttachment(context.Background(), "42/../../x", filename, outPath); err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if err := client.DeleteAttachment(context.Background(), "42", filename); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}

	want := []string{
		"GET /fmsg/42%2F..%2F..%2Fx/attach/my%20report%20%231%20100%25%3F.txt",
		"DELETE /fmsg/42/attach/my%20report%20%231%20100%25%3F.txt",
	}
	if len(rawPaths) != len(want) {
		t.Fatalf("paths = %q, want %q", rawPaths, want)
	}
	for i := range want {
		if rawPaths[i] != want[i] {
			t.Errorf("path[%d] = %q, want %q", i, rawPaths[i], want[i])
		}
	}
}

func TestAtomicDownloadPreservesExistingFileOnError(t *testing.T) {
	t.Run("http error leaves existing file untouched", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()

		dir := t.TempDir()
		existing := filepath.Join(dir, "existing.txt")
		if err := os.WriteFile(existing, []byte("precious data"), 0600); err != nil {
			t.Fatal(err)
		}

		client := New(srv.URL, StaticTokenProvider("token"))
		err := client.DownloadData(context.Background(), "99", false, existing)
		var ae *apiError
		if !errors.As(err, &ae) || ae.StatusCode != http.StatusNotFound {
			t.Fatalf("DownloadData err = %v, want 404 apiError", err)
		}
		err = client.DownloadAttachment(context.Background(), "99", "a.txt", existing)
		if !errors.As(err, &ae) || ae.StatusCode != http.StatusNotFound {
			t.Fatalf("DownloadAttachment err = %v, want 404 apiError", err)
		}

		got, err := os.ReadFile(existing)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "precious data" {
			t.Fatalf("existing file was modified: got %q", string(got))
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected no temp files left behind, got %d entries", len(entries))
		}
	})

	t.Run("truncated stream cleans up temp file and preserves existing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Abruptly hijack and close to simulate a broken mid-stream download.
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
		}))
		defer srv.Close()

		dir := t.TempDir()
		existing := filepath.Join(dir, "keep.bin")
		if err := os.WriteFile(existing, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}

		client := New(srv.URL, StaticTokenProvider("token"))
		if err := client.DownloadAttachment(context.Background(), "7", "doc.bin", existing); err == nil {
			t.Fatal("expected error on truncated stream")
		}

		got, err := os.ReadFile(existing)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "original" {
			t.Fatalf("existing file corrupted: got %q", string(got))
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected temp file to be removed, got %d entries", len(entries))
		}
	})
}

func TestEditMessageSendsDataAndReportsWhetherSent(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/7/edit" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":9,"time":12.5,"sha256":"ab"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":null,"time":null,"sha256":null}`))
	}))
	defer srv.Close()

	client := New(srv.URL, &sequenceTokenProvider{})
	text := "fixed"
	edited, err := client.EditMessage(context.Background(), 7, &text)
	if err != nil {
		t.Fatal(err)
	}
	if !edited.Sent || edited.ID == nil || *edited.ID != 9 {
		t.Fatalf("edit result = %+v", edited)
	}
	reverted, err := client.EditMessage(context.Background(), 7, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.Sent || reverted.ID != nil {
		t.Fatalf("no-op revert result = %+v", reverted)
	}
	if bodies[0] != `{"data":"fixed"}` || bodies[1] != `{"data":null}` {
		t.Fatalf("request bodies = %q", bodies)
	}
}

// TestEditFieldsDecodeAndEditMessagesAreHidden pins the FMSG-008 fields on
// list items and that VisibleMessages drops the edit messages themselves.
func TestEditFieldsDecodeAndEditMessagesAreHidden(t *testing.T) {
	const body = `[
		{"id": 9, "has_pid": true, "pid": 7, "from": "@alice@example.com", "edit": true, "edited": null},
		{"id": 8, "from": "@bob@example.com", "edit": false, "edited": {"id": 12, "time": 3.5, "size": null}},
		{"id": 7, "from": "@alice@example.com", "edit": false, "edited": {"id": 9, "time": 2.5, "size": 5}}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	items, err := client.ListMessages(context.Background(), 20, 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(items) != 3 || !items[0].Edit {
		t.Fatalf("items = %+v", items)
	}
	if e := items[1].Edited; e == nil || e.ID != 12 || e.Size != nil {
		t.Fatalf("unappliable edit = %+v", e)
	}
	if e := items[2].Edited; e == nil || e.ID != 9 || e.Time != 2.5 || e.Size == nil || *e.Size != 5 {
		t.Fatalf("edited = %+v", e)
	}

	visible := VisibleMessages(items)
	if len(visible) != 2 || visible[0].ID != 8 || visible[1].ID != 7 {
		t.Fatalf("visible = %+v", visible)
	}
}

func TestDownloadDataRequestsTheOriginalOnlyWhenAsked(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fmsg/7/data" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	for _, original := range []bool{false, true} {
		if err := client.DownloadDataToWriter(context.Background(), "7", original, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if len(queries) != 2 || queries[0] != "" || queries[1] != "original=true" {
		t.Fatalf("queries = %q", queries)
	}
}

func TestDownloadToFilePermissionsAndSpecialTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("downloaded-bytes"))
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))

	t.Run("new file respects process umask", func(t *testing.T) {
		dir := t.TempDir()
		outPath := filepath.Join(dir, "new.txt")
		if err := client.DownloadAttachment(context.Background(), "7", "new.txt", outPath); err != nil {
			t.Fatalf("DownloadAttachment: %v", err)
		}
		got, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "downloaded-bytes" {
			t.Fatalf("content = %q, want %q", string(got), "downloaded-bytes")
		}
		if runtime.GOOS != "windows" {
			refPath := filepath.Join(dir, "ref.txt")
			ref, err := os.OpenFile(refPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
			if err != nil {
				t.Fatal(err)
			}
			_ = ref.Close()
			refInfo, err := os.Stat(refPath)
			if err != nil {
				t.Fatal(err)
			}
			outInfo, err := os.Stat(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if outInfo.Mode().Perm() != refInfo.Mode().Perm() {
				t.Fatalf("new file mode = %o, want umask-derived %o (not forced 0600)", outInfo.Mode().Perm(), refInfo.Mode().Perm())
			}
		}
	})

	t.Run("existing regular file preserves mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX file modes not applicable on Windows")
		}
		for _, wantMode := range []os.FileMode{0644, 0640} {
			dir := t.TempDir()
			outPath := filepath.Join(dir, "existing.txt")
			if err := os.WriteFile(outPath, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(outPath, wantMode); err != nil {
				t.Fatal(err)
			}
			if err := client.DownloadAttachment(context.Background(), "7", "existing.txt", outPath); err != nil {
				t.Fatalf("DownloadAttachment (mode %o): %v", wantMode, err)
			}
			got, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "downloaded-bytes" {
				t.Fatalf("content = %q, want %q", string(got), "downloaded-bytes")
			}
			info, err := os.Stat(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != wantMode {
				t.Fatalf("overwritten file mode = %o, want %o", info.Mode().Perm(), wantMode)
			}
		}
	})

	t.Run("symlink destination writes through without replacing symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks may require elevated privileges on Windows")
		}
		dir := t.TempDir()
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link.txt")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := client.DownloadAttachment(context.Background(), "7", "a.txt", link); err != nil {
			t.Fatalf("DownloadAttachment through symlink: %v", err)
		}
		linfo, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if linfo.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("expected %s to remain a symlink, got mode %v", link, linfo.Mode())
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "downloaded-bytes" {
			t.Fatalf("target content = %q, want %q", string(got), "downloaded-bytes")
		}
	})

	t.Run("dev null works as destination", func(t *testing.T) {
		if err := client.DownloadAttachment(context.Background(), "7", "a.txt", os.DevNull); err != nil {
			t.Fatalf("DownloadAttachment(%s): %v", os.DevNull, err)
		}
	})

	t.Run("writable file in read-only directory falls back to direct write", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("requires non-root POSIX environment")
		}
		dir := filepath.Join(t.TempDir(), "ro")
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		outPath := filepath.Join(dir, "writable.txt")
		if err := os.WriteFile(outPath, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

		if err := client.DownloadData(context.Background(), "7", false, outPath); err != nil {
			t.Fatalf("DownloadData into read-only dir: %v", err)
		}
		got, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "downloaded-bytes" {
			t.Fatalf("content = %q, want %q", string(got), "downloaded-bytes")
		}
	})
}

func TestInlineBodyRequestsAreNotBoundedByRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fmsg":
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPut && r.URL.Path == "/fmsg/1":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/fmsg/1/edit":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2,"time":1.0,"sha256":"abc"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/fmsg/1":
			_, _ = w.Write([]byte(`{"from":"@a@example.com"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, StaticTokenProvider("token"))
	client.RequestTimeout = 20 * time.Millisecond

	if _, err := client.CreateMessage(context.Background(), []byte(`{"data":"big"}`)); err != nil {
		t.Fatalf("CreateMessage should not be cut off by RequestTimeout: %v", err)
	}
	if err := client.UpdateMessage(context.Background(), 1, []byte(`{"data":"big"}`)); err != nil {
		t.Fatalf("UpdateMessage should not be cut off by RequestTimeout: %v", err)
	}
	body := "big"
	if _, err := client.EditMessage(context.Background(), 1, &body); err != nil {
		t.Fatalf("EditMessage should not be cut off by RequestTimeout: %v", err)
	}
	if _, err := client.GetMessage(context.Background(), "1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetMessage err = %v, want context.DeadlineExceeded", err)
	}
}
