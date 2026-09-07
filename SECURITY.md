# Security Policy

## Supported Versions

The Fabric Smart Client is developed on `main` and shipped as tagged releases (see
[Versioning](README.md#versioning)). There are no maintenance branches: fixes land on
`main` and go out in the next release. Only the **latest release** receives security
fixes, so please confirm a report against it before filing.

## Reporting a Vulnerability

**Do not open a public issue for a security vulnerability.** Use one of these private
channels instead:

1. **GitHub private vulnerability reporting** (preferred) — file it directly at
   [Report a vulnerability](https://github.com/hyperledger-labs/fabric-smart-client/security/advisories/new).
   The report, the discussion, and the resulting advisory stay in one place, visible only
   to you and the maintainers.
2. **Email the LFDT security team** at
   [security@lists.hyperledger.org](mailto:security@lists.hyperledger.org) — use this if
   you cannot use GitHub, or if the issue affects several LFDT projects.

Please include as much of the following as you have:

- a description of the flaw and the impact you believe it has,
- the affected release or commit,
- steps, a test, or a minimal program that reproduces it,
- any mitigation or fix you would suggest.

## What to Expect

| Step | Timeline |
|------|----------|
| Acknowledgement that we received your report | within **14 days**, usually much sooner |
| Assessment — accepted, rejected, or needs more detail | in the private channel, after acknowledgement |
| Fix, release, and public advisory | coordinated with you before anything is published |

Confirmed vulnerabilities are fixed on `main`, released in a tagged version, and published
as a
[GitHub Security Advisory](https://github.com/hyperledger-labs/fabric-smart-client/security/advisories),
with a CVE where one applies. We credit reporters in the advisory unless you ask us not
to, and nothing about the report is made public before the fix is available.

The broader process the LFDT security team follows is documented on the
[Defect Response](https://lf-hyperledger.atlassian.net/wiki/spaces/SEC/pages/20283618/Defect+Response)
page.

## Scope

FSC is a client-side framework and has **not been formally audited** (see the
[disclaimer](README.md#disclaimer-and-license)). In scope: FSC's own code, its default
configuration, and its handling of keys, identities, sessions, and TLS. Vulnerabilities in
Hyperledger Fabric, Fabric-x, or another dependency belong to that project and should be
reported there — if you are unsure which, report it here and we will route it.
