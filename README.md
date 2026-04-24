# fmsg-cli

Command-line interface to [fmsg-webapi](https://github.com/markmnl/fmsg-webapi) fronting a fmsgd instance.

## Requirements

- Go 1.24 or newer

## Build

```sh
go build -o fmsg
```

## Usage

### Authentication

Before using any other command, log in:

```sh
fmsg login [address]
```

You can optionally provide the fmsg address directly (e.g. `@user@example.com`) to skip the prompt:

```sh
fmsg login @user@example.com
```

If the provided value contains no `@` symbols (argument or prompted input), it is treated as just the user part and expanded to `@<user>@<domain>` using the configured `FMSG_API_URL` domain.

If no address argument is provided, you will be prompted interactively. A JWT token is generated locally and stored in `$XDG_CONFIG_HOME/fmsg/auth.json` (typically `~/.config/fmsg/auth.json`) with `0600` permissions. The token is valid for 24 hours.

### Configuration

If a `.env` file exists in the working directory it is loaded automatically on startup (see `.env.example`). Environment variables set in the shell take precedence over values in `.env`.

| Variable      | Default                  | Description               |
|---------------|--------------------------|---------------------------|
| `FMSG_API_URL` | `http://127.0.0.1:8000` | Base URL of the fmsg-webapi |
| `FMSG_JWT_SECRET` | *(required)* | Secret used to sign JWT tokens (must match the server) |

`FMSG_JWT_SECRET` formats:
- Plain string (used as-is): `FMSG_JWT_SECRET=super-secret`
- Base64 with `base64:` prefix (decoded to raw bytes): `FMSG_JWT_SECRET=base64:c3VwZXItc2VjcmV0`

### Commands

| Command | Description |
|---------|-------------|
| `fmsg login [address]` | Authenticate and store a local token (optional address argument) |
| `fmsg list` \| `fmsg ls [--limit N] [--offset N]` | List messages for the authenticated user |
| `fmsg sent [--limit N] [--offset N]` | List messages authored by the authenticated user |
| `fmsg wait [--since-id N] [--timeout N]` | Long-poll for new messages |
| `fmsg get <message-id>` | Retrieve a message by ID |
| `fmsg send <recipient> <file\|text\|->` | Send a message (file path, text, or `-` for stdin) |
| `fmsg draft create <recipient> <file\|text\|->` | Create a draft message without sending |
| `fmsg draft send <message-id>` | Send a previously created draft |
| `fmsg update <message-id> [file\|text\|->` | Update a draft message |
| `fmsg del <message-id>` | Delete a draft message by ID |
| `fmsg add-to <message-id> <recipient> [recipient...]` | Add additional recipients to a message |
| `fmsg attach <message-id> <file>` | Upload a file attachment to a message |
| `fmsg get-attach <message-id> <filename> <output-file>` | Download an attachment |
| `fmsg get-data <message-id> [output-file]` | Download message body data (stdout if no output file) |
| `fmsg rm-attach <message-id> <filename>` | Remove an attachment from a message |

### Examples

```sh
# Login
fmsg login
fmsg login @user@example.com

# List messages
fmsg list
fmsg list --limit 10 --offset 20

# List authored messages (sent + drafts)
fmsg sent
fmsg sent --limit 10 --offset 20

# Wait for a new message
fmsg wait
fmsg wait --since-id 120 --timeout 10

# Get a specific message
fmsg get 101

# Send a message
fmsg send @recipient@example.com "Hello, world!"
fmsg send @recipient@example.com ./message.txt
echo "Hello via stdin" | fmsg send @recipient@example.com -

# Reply to an existing message
fmsg send --pid 12345 @recipient@example.com "hey there!"

# Send with optional flags
fmsg send --topic "Project update" --important @recipient@example.com ./update.txt
fmsg send --no-reply @recipient@example.com "Do not reply to this"

# Create/send a draft in two steps
fmsg draft create @recipient@example.com "Draft body"
fmsg update 42 --topic "Final topic"
fmsg attach 42 ./report.pdf
fmsg draft send 42

# Add additional recipients to a message
fmsg add-to 101 @other@example.com
fmsg add-to 101 @cc1@example.com @cc2@example.com

# Update a draft message
fmsg update 42 --topic "New topic"
fmsg update 42 --to @newrecipient@example.com "Updated body text"
fmsg update 42 --important

# Delete a draft message
fmsg del 101

# Upload attachment
fmsg attach 101 ./report.pdf

# Download attachment
fmsg get-attach 101 report.pdf ./downloaded-report.pdf

# Download message body data
fmsg get-data 101
fmsg get-data 101 ./message-body.txt

# Remove attachment
fmsg rm-attach 101 report.pdf
```
