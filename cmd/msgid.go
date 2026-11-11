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
// Edit messages (FMSG-008) are not counted, matching what "list" shows.
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

	// Negative index: walk the inbox list (ordered by ID desc) a page at a
	// time, counting only the messages "list" shows. -1 is the first, etc.
	const pageSize = 100
	remaining := int(-id)
	for offset := 0; ; offset += pageSize {
		page, err := client.ListMessages(pageSize, offset)
		if err != nil {
			return 0, fmt.Errorf("resolving message index %d: %w", id, err)
		}
		visible := api.VisibleMessages(page)
		if remaining <= len(visible) {
			return visible[remaining-1].ID, nil
		}
		remaining -= len(visible)
		if len(page) < pageSize {
			return 0, fmt.Errorf("no message at index %d", id)
		}
	}
}
