# Security Policy

The ClamAV Operator takes security seriously. This document describes how to
report vulnerabilities and what to expect.

## Supported versions

| Version | Supported |
|---------|-----------|
| 0.8.x   | ✅        |
| < 0.8   | ❌        |

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
discussions, or pull requests.**

Instead, report them privately via
[GitHub private vulnerability reporting](https://github.com/SolucTeam/clamav-operator/security/advisories/new)
(preferred), or by email to **k.aziz.k@live.fr** with the subject
`[clamav-operator] Security disclosure`.

Please include as much of the following as possible:

- Type of issue (e.g. privilege escalation, SSRF, token leak, path traversal)
- Affected version(s) and component (operator, scanner, Helm chart)
- Step-by-step reproduction or proof of concept
- Impact assessment

## What to expect

- **Acknowledgment** within 72 hours
- **Initial assessment** (severity, affected versions) within 7 days
- **Fix and coordinated disclosure** — we aim to ship a patch release and a
  GitHub Security Advisory (with CVE if applicable) once a fix is available.
  Reporters are credited unless they prefer anonymity.

## Security posture of the project

The project applies the following measures, verified in CI on every release:

- Images scanned with **Trivy**; SBOM (SPDX) attached to releases
- Images **signed with cosign** (keyless)
- Least-privilege RBAC; admission webhooks validating all CRs
- Notification URLs validated against SSRF; secrets never logged

If you believe any of these guarantees is broken, that is a security issue —
please report it as described above.
