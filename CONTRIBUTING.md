# Contributing to Dun

Thank you for your interest in contributing to Dun.

## Getting Started

```bash
git clone https://github.com/NICE-DEV226/Dun.git
cd dun
go build ./cmd/dun
go test ./...
```

## Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit with clear, descriptive messages
4. Push and open a Pull Request

## Code Standards

- Follow standard Go conventions and `gofmt`/`goimports` formatting
- All exported functions must have doc comments
- All packages must have tests
- Benchmarks are encouraged for performance-sensitive code
- No CGO dependencies in the core

## RFC Process

Significant architectural changes require an RFC before implementation.
See `rfcs/` for the template.

## Reporting Issues

Use GitHub Issues for bug reports and feature requests.
Include Go version, OS, and reproduction steps.

## License

By contributing, you agree that your contributions will be licensed
under the Apache License 2.0.
