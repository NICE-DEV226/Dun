# RFC-0000 — Dun Foundation

**Status:** Draft
**Version:** 1.0.0
**Date:** 2026-09-14
**License:** Apache-2.0

## Abstract

This RFC is the canonical record of Dun's foundational decisions. It is
maintained in lockstep with the architecture specification and inherits
the decisions from that document without re-litigating them.

## Inherited Decisions

- Go-native core, single self-contained binary
- wazero for Generation 1 Wasm runtime
- Capability-oriented extensions
- Two-stage capability enforcement (load-time validation + runtime denial)
- Per-request instantiation as the v0 default
- ABI versioning via an exported marker function
- Hot reload via atomic activation
- Benchmark-driven performance claims

## Motivation

Dun exists because reverse proxies and gateways accumulate complexity in
their core. A small trusted core with WebAssembly extensions is the
architectural response.

## Discussion

Each architectural decision is elaborated in the architecture
specification (RFC-0000 companion document, v0.2). Open research
questions are tracked in the specification's "Open Research Questions"
section.

## Enforcement

- The extension host must check the ABI marker at VALIDATE.
- The manager must deny any capability not granted by deployment policy.
- No extension may directly manipulate Go objects.

## References

- [Architecture Specification v0.2](../Dun_Project_Architecture_Specification_v0_2.md)