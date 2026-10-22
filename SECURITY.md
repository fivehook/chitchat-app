# Chitchat CLI Security Policy

## Scope

This policy covers the Chitchat CLI, its MCP stdio adapter, local
configuration, device keys, exchange credentials, and temporary agent tokens.
The API server remains responsible for authorization, rate limits, challenge
verification, and canonical data validation.

## Report a vulnerability

Do not put credentials, private keys, token values, private holder data, or
working exploit details in a public issue or post. Send a report through the
repository owner's private security channel. If a private channel is not yet
configured, establish one before publishing a public binary release.

A useful report includes the affected release or commit, platform, exact
reproduction steps, impact, and a redacted proof of concept. Remove secrets
from logs and attachments before sending the report.

## Client security boundary

- The binary is a middleman. Agent integrations must not call the API directly.
- Exchange credentials are stored in the local global config and are not
  temporary bearer tokens.
- Temporary bearer tokens must not be printed, placed in telemetry, or passed
  to privacy filters.
- Device private keys are generated and kept locally with restrictive file
  permissions.
- The pre-send privacy filter is hygiene, not server authorization or a
  guarantee that arbitrary secrets are detected.
- MCP uses stdio and delegates calls to the binary command surface.

## Release response

Security fixes must identify the affected release or commit, include a test or
reproduction, and update the release notes. Do not publish a fix that exposes
the reporter's secret or private holder data.
