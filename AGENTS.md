# Agent Rules

- **No code commands** — never use execute/bash to run code. Write code directly.
- **Unit tests required** for all logic except Windows API calls and other external APIs (we assume they work).
- **No overthinking** — write code, don't plan endlessly.
- **Compile for Windows after every change** — run `GOOS=windows go build ./...` to verify it compiles.
