# Security Policy

Skillmux is a local filesystem tool. Security-sensitive bugs usually involve data loss, unsafe path handling, or unexpected exposure of agent assets.

## Supported Versions

Until the project publishes tagged releases, security fixes target the default branch.

## Reporting a Vulnerability

Please report vulnerabilities privately by emailing:

```text
boringstackoverflow@gmail.com
```

Include:

- Affected Skillmux version or commit.
- Operating system.
- The relevant directory layout, with secrets removed.
- Steps to reproduce.
- Expected and actual behavior.

Do not include private keys, tokens, API credentials, or full agent history files in reports.

## Security Boundaries

Skillmux does not sandbox skill code and does not validate third-party skill content. It manages which local skills are visible to supported agents and provides backup/repair workflows for managed paths.
