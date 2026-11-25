//go:build unix

package cmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
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

	got, err := readMessageContent(fifo)
	if err != nil {
		t.Fatalf("readMessageContent(fifo): %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writing fifo: %v", err)
	}
	if string(got) != "from named pipe" {
		t.Fatalf("got %q, want %q", string(got), "from named pipe")
	}
}
