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
fmsg login
```

You will be prompted for your FMSG address (e.g. `@user@example.com`). A JWT token is generated locally and stored in `$XDG_CONFIG_HOME/fmsg/auth.json` (typically `~/.config/fmsg/auth.json`) with `0600` permissions. The token is valid for 24 hours.

### Configuration

| Variable      | Default                  | Description               |
|---------------|--------------------------|---------------------------|
| `FMSG_API_URL` | `http://localhost:4930` | Base URL of the fmsg-webapi |

### Commands

| Command | Description |
|---------|-------------|
| `fmsg login` | Authenticate and store a local token |
| `fmsg list [--limit N] [--offset N]` | List messages for the authenticated user |
| `fmsg get <message-id>` | Retrieve a message by ID |
| `fmsg send <recipient> <file\|text\|->` | Send a message (file path, text, or `-` for stdin) |
| `fmsg update <message-id> [file\|text\|->` | Update a draft message |
| `fmsg del <message-id>` | Delete a draft message by ID |
| `fmsg add-to <message-id> <recipient> [recipient...]` | Add additional recipients to a message |
| `fmsg attach <message-id> <file>` | Upload a file attachment to a message |
| `fmsg get-attach <message-id> <filename> <output-file>` | Download an attachment |
| `fmsg get-data <message-id> <output-file>` | Download the message body data |
| `fmsg rm-attach <message-id> <filename>` | Remove an attachment from a message |

### Examples

```sh
# Login
fmsg login

# List messages
fmsg list
fmsg list --limit 10 --offset 20

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
fmsg get-data 101 ./message-body.txt

# Remove attachment
fmsg rm-attach 101 report.pdf
```
