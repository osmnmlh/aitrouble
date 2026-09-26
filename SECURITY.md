# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| latest (`main`) | ✅ |
| older releases | ⚠️ Best-effort |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues via [GitHub's private vulnerability reporting](https://github.com/osmnmlh/aitrouble/security/advisories/new).

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact

We will respond within 72 hours.

## Security Design Principles

`aitrouble` is built with security as a first principle:

- **Read-only**: Never modifies config files or environment variables.
- **Secret redaction**: API keys are masked in all output (e.g., `sk-•••oXyZ`).
- **Zero telemetry**: No data leaves your machine.
- **No persistent storage**: No logs, caches, or databases are written.
- **Single binary**: No scripts with elevated permissions.
