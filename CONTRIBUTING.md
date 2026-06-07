# Contributing to ScrapeOwl

Thank you for your interest in contributing to ScrapeOwl! This document provides guidelines for contributing to the project.

## Code of Conduct

Be respectful and constructive. We're building something useful for the community.

## How to Contribute

### Reporting Bugs

1. Check existing issues before opening a new one
2. Use the bug report template
3. Include reproduction steps, expected vs actual behavior, and environment details

### Suggesting Features

1. Open a discussion issue first to gauge interest
2. Describe the use case and potential implementation
3. Be patient — we prioritize features that align with the open-core model

### Submitting Code

1. Fork the repository and create a feature branch
2. Write tests for your changes
3. Ensure all tests pass: `go test ./...`
4. Run `go vet ./...` with no errors
5. Format your code: `gofmt -w .`
6. Submit a pull request with a clear description

## Development Setup

```bash
# Clone
git clone https://github.com/ashleyfullero/scrapeowl
cd scrapeowl

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Start the development server
make run
```

## Project Structure

```
cmd/scrapeowl/     - CLI entry point
internal/config/   - YAML config parsing (add fields here for new job options)
internal/browser/  - Chrome automation (add new actions here)
internal/extractor/- Data extraction (add new extractor types here)
internal/runner/   - Job orchestration
internal/api/      - REST API handlers
web/               - Frontend dashboard
```

## Adding a New Browser Action

1. Add the action name to `validateStep()` in `internal/config/config.go`
2. Add a case in `browser.RunStep()` in `internal/browser/browser.go`
3. Add an example to `examples/` directory
4. Add a test in `internal/config/config_test.go`

## Adding a New Extractor Type

1. Add the type to `validateExtractor()` in `internal/config/config.go`
2. Add a handler function in `internal/extractor/extractor.go`
3. Add a test case

## Code Style

- Use `gofmt` for formatting
- Follow standard Go idioms
- Prefer explicit error handling over panics
- Write descriptive variable names
- Add comments for exported functions

## Commit Messages

Use conventional commits format:
- `feat:` — new features
- `fix:` — bug fixes  
- `docs:` — documentation changes
- `test:` — test changes
- `refactor:` — code restructuring
- `chore:` — dependency updates, build changes

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
