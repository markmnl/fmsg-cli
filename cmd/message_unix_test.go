//go:build unix

package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadMessageContentFromFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "msg.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	writeErr := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			writeErr <- err
			return
		}
		_, werr := f.WriteString("from named pipe")
		cerr := f.Close()
		if werr != nil {
			writeErr <- werr
			return
		}
		writeErr <- cerr
	}()

	got, err := readMessageContent(context.Background(), fifo)
	if err != nil {
		t.Fatalf("readMessageContent(context.Background(), fifo): %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writing fifo: %v", err)
	}
	if string(got) != "from named pipe" {
		t.Fatalf("got %q, want %q", string(got), "from named pipe")
	}
}

// A read that blocks (here: a named pipe nobody has opened for writing, which
// behaves like a terminal's stdin with nothing typed) must be abandoned when
// the command context is cancelled, or Ctrl-C would not stop the command.
func TestReadMessageContentReturnsWhenContextCancelled(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "silent.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	// Unblock the abandoned reader once the test is over.
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := readMessageContent(ctx, fifo)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		t.Fatalf("readMessageContent returned before cancellation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readMessageContent still blocked after the context was cancelled")
	}
}
