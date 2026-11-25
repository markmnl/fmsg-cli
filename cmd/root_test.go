package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func newInterruptTestRoot(runErr func(ctx context.Context) error) (*cobra.Command, *bytes.Buffer) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{
		Use:  "work",
		RunE: func(c *cobra.Command, args []string) error { return runErr(c.Context()) },
	})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"work"})
	silenceOnInterrupt(root)
	return root, &out
}

func TestSilenceOnInterruptSuppressesCancellationOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root, out := newInterruptTestRoot(func(ctx context.Context) error {
		return fmt.Errorf("reading stdin: %w", ctx.Err())
	})

	err := root.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output after an interrupt, got %q", out.String())
	}
}

func TestSilenceOnInterruptKeepsOrdinaryErrors(t *testing.T) {
	root, out := newInterruptTestRoot(func(context.Context) error {
		return errors.New("boom")
	})

	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if !bytes.Contains(out.Bytes(), []byte("boom")) {
		t.Fatalf("ordinary errors must still be reported, got %q", out.String())
	}
}
