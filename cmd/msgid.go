package cmd

import (
	"fmt"
	"strconv"

	"github.com/markmnl/fmsg-cli/internal/api"
)

// resolveMessageID parses arg as a message ID. Positive values are returned
// as-is. Negative values are treated as reverse indices into the user's inbox
// ordered by ID descending: -1 is the most recent message, -2 the second most
// recent, and so on. Zero is not a valid message ID and returns an error.
//
// When arg is negative the caller's inbox is fetched via GET /fmsg using the
// provided client, so the client must already be authenticated.
func resolveMessageID(client *api.Client, arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid message ID %q: %w", arg, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("invalid message ID: 0 is not a valid message ID")
	}
	if id > 0 {
		return id, nil
	}

	// Negative index: resolve via inbox list (ordered by ID desc).
	// -1 → offset 0, -2 → offset 1, etc.
	offset := int(-id) - 1
	msgs, err := client.ListMessages(1, offset)
	if err != nil {
		return 0, fmt.Errorf("resolving message index %d: %w", id, err)
	}
	if len(msgs) == 0 {
		return 0, fmt.Errorf("no message at index %d", id)
	}
	return msgs[0].ID, nil
}
