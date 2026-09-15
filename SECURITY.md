# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Dun, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email: nicedev226@gmail.com

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response Timeline

- Acknowledgment: within 48 hours
- Assessment: within 1 week
- Fix or mitigation: based on severity

## Scope

This security policy covers:
- The Dun core runtime
- The Wasm extension host
- The capability enforcement system
- Configuration parsing
- The plugin loading pipeline

## Out of Scope

- Third-party Wasm plugins
- Upstream services proxied by Dun
- User configurations that intentionally weaken security

## Security Model

See [ARCHITECTURE.md](docs/ARCHITECTURE.md) for details on:
- Trusted Computing Base
- Capability model
- Two-stage enforcement
- Plugin sandboxing
