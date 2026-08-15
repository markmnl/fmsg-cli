package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/spf13/cobra"
)

var (
	watchEvents  []string
	watchOnce    bool
	watchTimeout time.Duration
)

// exitNoEvent is the exit code when --once/--timeout ends without an event —
// distinct from 1 (error) so scripts can tell "nothing arrived" from failure.
const exitNoEvent = 2

// errNoEvent is returned by watch when it ends without having printed an
// event; Execute maps it to exitNoEvent.
var errNoEvent = errors.New("no event received")

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stream new-message notifications over the server's WebSocket",
	Long: `Connect to the fmsg-webapi WebSocket and print each pushed event as it
arrives, until interrupted (Ctrl-C), --once, or --timeout.

Events pushed by the server: new_msg (a message arrived for you), delivered
(a message you sent reached a recipient), recipients_added. Filter with
--events; every event carries the message in the same shape as a "list" item.

With --json each event is one JSON line: {"type":"new_msg","data":{...}}.
A {"type":"ready"} line is printed once the socket is open — and again after
every reconnect, since events may have been missed while it was down; do a
"list" catch-up when you see it. The connection is redialled automatically
if it drops.

Exit codes: 0 after an event (--once) or when stopped by Ctrl-C/--timeout;
2 if --once/--timeout ended before any event; 1 on error.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if watchTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, watchTimeout)
			defer cancel()
		}

		wanted := map[string]bool{}
		for _, e := range watchEvents {
			for _, part := range strings.Split(e, ",") {
				if part = strings.TrimSpace(part); part != "" {
					wanted[part] = true
				}
			}
		}

		printed := 0
		opts := api.WatchOptions{
			Reconnect: true,
			OnConnect: func() {
				if jsonOutput {
					_ = printJSON(map[string]string{"type": "ready"})
				} else {
					fmt.Fprintf(os.Stderr, "watching %s (Ctrl-C to stop)\n", client.WatchURL())
				}
			},
		}
		err := client.Watch(ctx, opts, func(ev api.WatchEvent) error {
			if len(wanted) > 0 && !wanted[ev.Type] {
				return nil
			}
			if jsonOutput {
				if err := printJSON(ev); err != nil {
					return err
				}
			} else {
				printHumanEvent(ev)
			}
			printed++
			if watchOnce {
				return api.ErrStopWatch
			}
			return nil
		})

		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// Ctrl-C or --timeout: fine unless the caller wanted an event.
			if printed == 0 && errors.Is(err, context.DeadlineExceeded) {
				return errNoEvent
			}
			return nil
		default:
			return err
		}
	},
}

// printHumanEvent renders one event as a single line.
func printHumanEvent(ev api.WatchEvent) {
	item, err := ev.Item()
	if err != nil {
		fmt.Printf("%s  %s\n", ev.Type, strings.TrimSpace(string(ev.Data)))
		return
	}
	to, _ := json.Marshal(item.To)
	line := fmt.Sprintf("%-16s ID: %d  From: %s  To: %s", ev.Type, item.ID, item.From, string(to))
	if item.Topic != "" {
		line += fmt.Sprintf("  Topic: %q", item.Topic)
	}
	fmt.Println(line)
}

func init() {
	watchCmd.Flags().StringSliceVar(&watchEvents, "events", nil, "Only print these event types (comma-separated: new_msg,delivered,recipients_added); default all")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "Exit after the first matching event")
	watchCmd.Flags().DurationVar(&watchTimeout, "timeout", 0, "Stop after this duration (e.g. 30s, 5m); 0 means run until interrupted")
	rootCmd.AddCommand(watchCmd)
}
