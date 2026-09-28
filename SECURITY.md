# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| `main` | ✅ Active development |
| `v0.1.x` | ✅ Current stable |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues via [GitHub's private vulnerability reporting](https://github.com/osmnmlh/aitrouble/security/advisories/new).

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact

We will review reports promptly. Response time depends on severity and maintainer availability.

## Security Design Principles

`aitrouble` is designed to be safe to run against production AI configurations:

- **Read-only**: Never modifies config files, environment variables, or any file on disk.
- **Secret redaction**: API keys, tokens, and passwords are masked in all output as `[REDACTED]`. MCP environment variable values are never stored or printed.
- **URL redaction**: Sensitive query parameters (token, key, secret, password, auth...) and userinfo credentials in MCP HTTP URLs are redacted before display.
- **No command execution**: MCP server commands discovered in configuration files are never executed (M5A is static discovery only).
- **Zero telemetry**: We do not collect or send diagnostic analytics, crash reports, or user data to any centralized service. Network and provider probes only send HTTP/network requests to the exact targets configured in your local environment.
- **No persistent storage**: No logs, caches, or databases are written.
- **Single binary**: No install scripts with elevated permissions.
- **Bounded responses**: Provider responses are read with a size limit to prevent memory exhaustion.
