# Security policy

## Supported versions

Servy is pre-v1. Until the first public release, only the default branch is supported.

## Reporting vulnerabilities

Please do not open public issues for vulnerabilities that could help compromise servers.

Report privately through GitHub Security Advisories once the repository is public, or contact the maintainer directly if advisories are not yet enabled.

Include:

- affected commit or release;
- host OS and architecture;
- exact config and command used, with secrets removed;
- expected vs actual behavior;
- impact and reproduction steps.

## Security model

Servy runs locally on the server and may execute privileged commands. Its safety model relies on:

- explicit plans before mutation;
- strict config validation;
- confirmation gates for dangerous actions;
- official upstream installation sources;
- no silent SSH lockout-risk changes;
- preserving existing host configuration by default.

## Release integrity

Public releases must publish checksums and signed release artifacts before `install.sh` is advertised as the recommended installation method. Release tags should be protected and signed, and release workflows must run tests and vulnerability scans before uploading artifacts.
