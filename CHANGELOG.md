# Changelog

All notable changes to Dun will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

The next release will focus on external plugin authoring, CI hardening and
operational observability.

## [0.1.0] - 2026-09-16

### Added
- Public Core-Wasm v0 contract and conformance tests
- TinyGo and Rust SDK foundations
- Plugin ABI validation and `dun plugin inspect|validate`
- Health, readiness and Prometheus metrics endpoints
- Optional SHA-256 plugin artifact verification
- GitHub Actions checks for Go, Rust and ABI conformance
- Reproducible Wasm invocation benchmark

### Security
- Per-request Wasm instance isolation
- Capability-gated host imports
- Execution timeout and initial memory limits
- Reload draining and explicit plugin shutdown

### Added
- Initial project scaffolding and architecture
- HTTP server with reverse proxy
- Route matching with prefix and exact rules
- YAML-based configuration
- Extension pipeline abstraction
- CLI entry point (`dun run`)
- Structured logging
- Request metrics
- RFC-0000: Foundation document
- Apache-2.0 license
