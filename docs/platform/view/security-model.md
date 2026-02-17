# View Runtime Security Model

## Overview

The Fabric Smart Client (FSC) view runtime operates under a **shared process security model** where all registered views execute within the same process space without isolation. This document describes the current security model, its implications, and best practices for secure deployment.

## Security Model

### Trusted Computing Base (TCB)

**All view code registered at an FSC node is considered part of the Trusted Computing Base (TCB).** This means:

- View code has full access to the process memory space
- Views share the same execution context without isolation
- Malicious or compromised view code can affect the entire system
- **Once a view is loaded, it has complete access to all system resources**

### Lack of View Isolation

Unlike containerized or sandboxed execution environments, FSC views currently execute without isolation boundaries:

- **Shared Context**: Multiple views share the same `view.Context` object
- **Service Override Risk**: Malicious views can override critical services (signer, network services, etc.)
- **Cross-View Contamination**: Modifications made by one view persist and affect all other views
- **No Resource Limits**: Views are not restricted in CPU, memory, or I/O usage

### Dependency Chain Trust

The security model extends beyond your own view code:

> **"Once your dependencies are in, they are in. While I may trust your code, I may not trust your dependencies."**

- All transitive dependencies become part of the TCB
- Compromised dependencies can exploit the same vulnerabilities as malicious view code
- Supply chain attacks are a significant concern

## Best Practices

### 1. Strict View Code Review

- **Audit all view code** before deployment
- Implement code review processes for all views
- Never deploy views from untrusted sources

### 2. Dependency Management

- **Audit all dependencies** and their transitive dependencies
- Use dependency scanning tools (e.g., `go mod graph`, vulnerability scanners)
- Pin dependency versions to prevent supply chain attacks
- Regularly update dependencies to patch known vulnerabilities
- Consider vendoring dependencies for reproducible builds

### 3. Access Control

- Use separate FSC nodes for different trust domains
- Never mix trusted and untrusted views on the same node

## Conclusion

The current FSC view runtime security model requires **complete trust in all deployed view code and dependencies**. Developers must implement strict governance and access controls to ensure only trusted code executes on FSC nodes. 

**Key Takeaway**: Treat view deployment with the same security rigor as deploying system-level code or kernel modules. Once malicious code enters the process space, the entire node is compromised.

---

*This security model documentation reflects the current state of FSC. Always check the latest releases and security advisories for updates.*
