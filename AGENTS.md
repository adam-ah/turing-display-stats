# Agent Rules

- Bugs to be reproduced by unit tests first
- Unit tests required for all logic except Windows API calls and other external APIs (we assume they work).
- No overthinking — write code, don't plan endlessly.
- Run `gofmt` on changed Go files after every edit.
- Lint the code after every change.
- Compile for Windows after every change — run `GOOS=windows go build ./...` to verify it compiles.
- Separate concerns - if a file gets too long or too messy, split it into multiple files.
