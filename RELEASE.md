# Chitchat CLI Release Checklist

This checklist applies before distributing a public binary. It does not
authorize a production deployment or change any server-side release state.

## Source and review

- Review the public CLI and MCP surface against the approved service contract.
- Confirm no API core, exchange credential, temporary token, or private key is
  embedded in source, examples, logs, or test fixtures.
- Run the privacy filter tests and inspect any new filter or plugin.
- Run the MCP compatibility tests. MCP must continue to delegate through the
  binary middleman.
- Review `SECURITY.md` and configure a private report channel before release.

## Reproducible build

Use the same clean source tree, Go toolchain, `GOOS`, `GOARCH`, and
`CGO_ENABLED=0` for both builds. Record `go version`, module files, target,
and the resulting SHA-256 checksum. The build helper uses `-trimpath`,
`-buildvcs=false`, and no timestamp or host path injection.

```bash
bash scripts/reproducible-build.sh dist/chitchat-release
```

The helper builds twice and fails if the bytes differ. The output directory
contains the binary and a `.sha256` file. Verify an artifact with:

```bash
sha256sum --check chitchat-linux-amd64.sha256
```

## Signing and publication

- Sign the checksum manifest with the release operator's approved signing key.
- Publish the binary, checksum manifest, signature, source revision, and Go
  toolchain version together.
- Keep signing keys outside the repository and outside agent configuration.
- Verify the published checksum and signature from a clean environment.
- Attach release notes with security changes and known limitations.

The repository does not contain a release signing key. A release is not
considered signed because a local build succeeded.
