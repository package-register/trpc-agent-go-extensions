# AGENTS.md — Engineering Protocol for trpc-agent-go-extensions

This file defines the default working protocol for coding agents (and humans) in this repository.
Scope: `trpc-agent-go-extensions` repository.

## 1) Core Philosophy: "Less is More"

Adhere to Go's core philosophy:

- **Simplicity**: Do not multiply entities beyond necessity (如无必要勿增实体).
- **Composition over Inheritance**: Use embedding and small interfaces.
- **Pragmatism**: Prioritize readable, maintainable code over "clever" abstractions.

## 2) Engineering Rules (Mandatory)

### 2.1 Interface Ownership

- **Rule**: Interfaces should be defined by the **consumer**, not the producer.
- **Exception**: Shared domain interfaces (e.g., `logger.Logger`) can live in a common package.
- **Action**: When adding a dependency, use an interface to decouple. Do not force high-level packages to depend on low-level implementation details.

### 2.2 Constructor Pattern

- **Rule**: "Accept interfaces, return structs".
- **Signature**: `func NewXxx(dep DependencyInterface, ...) *Xxx`.
- **Error Handling**: If initialization can fail, return `(*Xxx, error)`. Never ignore errors in constructors.

### 2.3 Telemetry & Observability

- **Standard**: Follow standardized attribute naming for Langfuse/OpenTelemetry.
- **Requirement**: All agentic steps must be traced. Use the standard attributes defined in `telemetry/constants/attributes.go`.

### 2.4 Configuration

- **Rule**: No direct environment variable access inside business logic or constructors.
- **Pattern**: Inject configuration via a `Config` struct or functional options.

## 3) Repository Structure

- `flow/`: High-level orchestration patterns.
- `pipeline/`: Core execution framework.
- `step/`: Atomic units of work (formerly PromptFile).
- `telemetry/`: Standardized observability.
- `mcp/`: Model Context Protocol extensions.

## 4) Definition of Done (DoD)

1. **Tests**: Every new feature or bug fix must have unit tests.
2. **Lint**: Code must pass `golangci-lint` (or equivalent standard Go checks).
3. **Documentation**: Update README and package-level comments for public APIs.
4. **Telemetry**: Ensure relevant traces are emitted with standard attributes.
