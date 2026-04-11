# Agent Instructions

## API Reference

The canonical API reference for this project is the **fmsg-webapi README**:

> https://github.com/markmnl/fmsg-webapi/blob/main/README.md

Before implementing or modifying any CLI command that interacts with the API, read that document to understand the exact routes, request/response shapes, query parameters, status codes, and authorization requirements.

## Project Overview

This is `fmsg-cli`, a Go CLI client for the fmsg HTTP API. It is built with [Cobra](https://github.com/spf13/cobra) and organized as follows:

- `cmd/` — one file per CLI command, each wiring Cobra flags to API calls via `internal/api`
- `internal/api/client.go` — HTTP client wrapping all API routes
- `internal/auth/` — JWT token storage and retrieval
- `internal/config/` — configuration loading

## Key Conventions

- All API routes require `Authorization: Bearer <token>`; the token is loaded via `internal/auth`
- API base URL is read from config (`internal/config`)
- Follow existing patterns in `cmd/` when adding new commands
- Go module path: see `go.mod`

## Documentation Maintenance

- Keep `README.md` concise and up to date.
- When CLI arguments, flags, or command behavior change, update `README.md` usage, command table, and relevant examples in the same change.
