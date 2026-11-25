package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// readMessageContent resolves a CLI message content argument:
//   - "-" reads all bytes from stdin.
//   - An existing non-directory path (regular file, symlink, named pipe /
//     process substitution, etc.) is read; read errors are returned rather
//     than silently falling back to the path as literal text.
//   - Anything else is used directly as literal text.
func readMessageContent(arg string) ([]byte, error) {
	if arg == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return data, nil
	}
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		data, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("reading file %q: %w", arg, err)
		}
		return data, nil
	}
	return []byte(arg), nil
}

type draftOptions struct {
	pid       int64
	hasPID    bool
	topic     string
	hasTopic  bool
	important bool
	noReply   bool
}

func buildDraftPayload(from, recipient string, data []byte, opts draftOptions) ([]byte, error) {
	msg := map[string]any{
		"from":    from,
		"to":      []string{recipient},
		"version": 1,
		"type":    "text/plain",
		"size":    len(data),
		"data":    string(data),
	}
	if opts.hasPID {
		msg["pid"] = opts.pid
	}
	if opts.hasTopic {
		msg["topic"] = opts.topic
	}
	if opts.important {
		msg["important"] = true
	}
	if opts.noReply {
		msg["no_reply"] = true
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encoding message: %w", err)
	}
	return payload, nil
}
