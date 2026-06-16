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

Before using any other command, log in with either a main-account user JWT or a sub-account API key:

```sh
fmsg login [api-key|jwt]
```

For main-account use, pass a JWT issued by the identity provider configured for your fmsg-webapi deployment:

```sh
fmsg login eyJ...
```

The JWT must contain the fmsg address claim expected by the server, commonly `fmsg_address`. If the token uses a deployment-specific claim that the CLI cannot recognize, pass the address explicitly:

```sh
fmsg login --address @user@example.com eyJ...
```

User JWTs are used directly until their JWT expiry. When they expire, run `fmsg login` again with a fresh JWT.

For sub-account or programmatic use, pass an opaque fmsg API key:

```sh
fmsg login fmsgk_<key_id>_<secret>
```

API keys are exchanged with `POST /fmsg/token` for short-lived first-party JWTs. The CLI caches the returned JWT and refreshes it automatically within five minutes of expiry. The default server token lifetime is 12 hours.

Credentials are stored in `$XDG_CONFIG_HOME/fmsg/auth.json` (typically `~/.config/fmsg/auth.json`) with `0600` permissions.

For non-interactive use, set `FMSG_API_KEY` instead of running `fmsg login`. Environment-provided API keys override stored credentials and are not written to disk.

### Configuration

If a `.env` file exists in the working directory it is loaded automatically on startup (see `.env.example`). Environment variables set in the shell take precedence over values in `.env`.

| Variable      | Default                  | Description               |
|---------------|--------------------------|---------------------------|
| `FMSG_API_URL` | `http://127.0.0.1:8000` | Base URL of the fmsg-webapi |
| `FMSG_API_KEY` | *(optional)* | Opaque API key used for non-interactive sub-account authentication |

Programmatic clients use API keys issued by fmsg-webapi.

### Commands

| Command | Description |
|---------|-------------|
| `fmsg login [api-key\|jwt] [--address @user@example.com]` | Authenticate and store credentials |
| `fmsg list` \| `fmsg ls [--limit N] [--offset N]` | List messages for the authenticated user |
| `fmsg sent [--limit N] [--offset N]` | List messages authored by the authenticated user |
| `fmsg get <message-id>` | Retrieve a message by ID, including the short text body for `text/*` messages |
| `fmsg send <recipient> <file\|text\|->` | Send a message (file path, text, or `-` for stdin) |
| `fmsg draft create <recipient> <file\|text\|->` | Create a draft message without sending |
| `fmsg draft send <message-id>` | Send a previously created draft |
| `fmsg update <message-id> [file\|text\|-]` | Update a draft message |
| `fmsg del <message-id>` | Delete a draft message by ID |
| `fmsg add-to <message-id> <recipient> [recipient...]` | Add additional recipients to a message |
| `fmsg attach <message-id> <file>` | Upload a file attachment to a message |
| `fmsg get-attach <message-id> <filename> <output-file>` | Download an attachment |
| `fmsg get-data <message-id> [output-file]` | Download message body data (stdout if no output file) |
| `fmsg rm-attach <message-id> <filename>` | Remove an attachment from a message |

Wherever a `<message-id>` is accepted you may supply a **negative index** to refer to a
recent message without knowing its ID.  The index is resolved against your inbox
(`GET /fmsg`), which is ordered by ID descending:

| Value | Meaning |
|-------|---------|
| `-1`  | Most recent message |
| `-2`  | Second most recent  |
| `-N`  | N-th most recent    |

Message creation commands support these optional flags:

| Command | Flags |
|---------|-------|
| `fmsg send` | `--pid, -p`, `--topic`, `--important`, `--no-reply` |
| `fmsg draft create` | `--pid, -p`, `--topic`, `--important`, `--no-reply` |
| `fmsg update` | `--to`, `--topic`, `--type`, `--pid, -p`, `--important`, `--no-reply` |

### Examples

```sh
# Login
fmsg login
fmsg login eyJ...
fmsg login --address @user@example.com eyJ...
fmsg login fmsgk_<key_id>_<secret>

# Non-interactive sub-account auth
FMSG_API_KEY=fmsgk_<key_id>_<secret> fmsg list

# List messages
fmsg list
fmsg list --limit 10 --offset 20

# List authored messages (sent + drafts)
fmsg sent
fmsg sent --limit 10 --offset 20

# Get a specific message
fmsg get 101

# Get the most recent message (negative index)
fmsg get -1

# Get the second most recent message
fmsg get -2

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
