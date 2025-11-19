# Kubernetes FolderTree Controller

**Transform complex RBAC management from hundreds of RoleBindings into a single, hierarchical resource.**

[![Tests](https://github.com/mhenriks/kubernetes-foldertree-controller/workflows/Tests/badge.svg)](https://github.com/mhenriks/kubernetes-foldertree-controller/actions)
[![E2E Tests](https://github.com/mhenriks/kubernetes-foldertree-controller/workflows/E2E%20Tests/badge.svg)](https://github.com/mhenriks/kubernetes-foldertree-controller/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/mhenriks/kubernetes-foldertree-controller)](https://goreportcard.com/report/github.com/mhenriks/kubernetes-foldertree-controller)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Container](https://img.shields.io/badge/ghcr.io-foldertree--controller-blue)](https://github.com/mhenriks/kubernetes-foldertree-controller/pkgs/container/foldertree-controller)

Managing Kubernetes RBAC at scale is painful. The FolderTree Controller solves this by organizing namespaces into tree structures with automatic permission inheritance - turning complex RBAC sprawl into simple, maintainable hierarchies.

## 🚀 Quick Links

| Getting Started | Documentation | Examples |
|----------------|---------------|----------|
| **[🏃 Quick Start](QUICKSTART.md)**<br/>Get running in 5 minutes | **[📖 Complete Guide](GUIDE.md)**<br/>Comprehensive documentation | **[🎯 Demo Examples](demo-examples/README.md)**<br/>Real-world scenarios |
| **[🏗️ Architecture](PROJECT_SUMMARY.md)**<br/>How it all works | **[🛡️ Security Model](GUIDE.md#security-model)**<br/>Privilege escalation prevention | **[🎤 Presentations](demo-examples/SLIDE_DECK.md)**<br/>Demo materials |

## The Problem We Solve

- 🔥 **RBAC Sprawl**: Managing 100+ RoleBindings across environments becomes unmanageable
- 🔥 **No Inheritance**: Manually duplicating permissions across related namespaces
- 🔥 **Error-Prone**: Easy to misconfigure permissions or create security gaps
- 🔥 **Audit Nightmare**: Understanding permission flows across your organization
- 🔥 **Maintenance Overhead**: Every team change requires updating dozens of RoleBindings

## Our Solution

✅ **One Resource**: Replace hundreds of RoleBindings with a single FolderTree
✅ **Automatic Inheritance**: Permissions flow naturally down organizational hierarchies
✅ **Secure by Default**: Comprehensive validation prevents privilege escalation
✅ **Production Ready**: Event-driven architecture with intelligent change detection
✅ **Selective Control**: Fine-grained `propagate` field controls what inherits where

## At a Glance

### Before: Traditional RBAC (6+ resources)
```yaml
# RoleBinding 1: platform-admin in prod-web
# RoleBinding 2: prod-ops in prod-web
# RoleBinding 3: web-developers in prod-web
# RoleBinding 4: platform-admin in prod-api
# RoleBinding 5: prod-ops in prod-api
# RoleBinding 6: api-developers in prod-api
# ... and many more manual RoleBindings
```

### After: FolderTree (1 resource)
```yaml
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: my-organization
spec:
  tree:
    name: platform
    subfolders:
    - name: production
      subfolders:
      - name: web-app
      - name: api-service
  folders:
  - name: platform
    roleBindingTemplates:
    - name: platform-admin
      propagate: true  # Inherits everywhere automatically
      subjects:
      - kind: Group
        name: platform-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
  - name: production
    roleBindingTemplates:
    - name: prod-ops
      propagate: true  # Inherits to production children
      subjects:
      - kind: Group
        name: production-operators
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
  - name: web-app
    roleBindingTemplates:
    - name: web-developers
      subjects:
      - kind: Group
        name: web-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["prod-web"]
  - name: api-service
    roleBindingTemplates:
    - name: api-developers
      subjects:
      - kind: Group
        name: api-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["prod-api"]
```

**Result**: Automatically creates all 6+ RoleBindings with proper inheritance:
- `prod-web` gets: `platform-admin` + `prod-ops` + `web-developers`
- `prod-api` gets: `platform-admin` + `prod-ops` + `api-developers`

## Key Features

### 🎯 **Hierarchical RBAC**
Organize namespaces into tree structures that mirror your organization. Permissions flow naturally from platform → environment → application.

### 🔄 **Selective Inheritance**
Fine-grained control with `propagate: true/false`. Platform admins inherit everywhere, but secrets access stays restricted to specific folders.

### 🛡️ **Security First**
Comprehensive validation prevents privilege escalation. Users can only grant permissions they already possess. Built-in admission webhook validates all operations.

### ⚡ **Event-Driven**
No polling. Responds immediately to changes with intelligent diff analysis - only updates what actually changed.

### 📊 **Production Ready**
Status reporting, health checks, metrics, automatic cleanup, and TLS webhooks. Built for enterprise environments.

### 🔧 **Namespace Lifecycle Aware**
Intelligently handles namespace deletion. Existing namespaces can be deleted without breaking FolderTrees. New namespaces must exist to prevent configuration errors.

## Installation

### Quick Install (Recommended)
Install the latest stable release directly from GitHub:
```bash
kubectl apply -f https://github.com/mhenriks/kubernetes-foldertree-controller/releases/latest/download/install.yaml
```

Or install a specific version:
```bash
VERSION=v0.1.0
kubectl apply -f https://github.com/mhenriks/kubernetes-foldertree-controller/releases/download/${VERSION}/install.yaml
```

The installation includes:
- ✅ Custom Resource Definitions (CRDs)
- ✅ Controller deployment with webhooks
- ✅ RBAC permissions
- ✅ TLS certificates (via cert-manager)

**Container Images**: Multi-platform images available at `ghcr.io/mhenriks/foldertree-controller`

### Development (Local)
```bash
git clone https://github.com/mhenriks/kubernetes-foldertree-controller
cd kubernetes-foldertree-controller
make install && ENABLE_WEBHOOKS=false make run
```

### Production (From Source)
```bash
git clone https://github.com/mhenriks/kubernetes-foldertree-controller
cd kubernetes-foldertree-controller
make deploy  # Includes CRDs, controller, webhooks, and RBAC
```

**Need help?** See the **[Quick Start Guide](QUICKSTART.md)** for detailed installation steps.

**Creating releases?** See the **[Release Guide](RELEASE.md)** for maintainers.

## Why Teams Choose FolderTree

- **90% Reduction** in RBAC resource management overhead
- **Zero Manual RoleBindings** - everything automated through inheritance
- **Audit-Ready** - clear permission inheritance paths
- **Security Improved** - built-in privilege escalation prevention
- **GitOps Friendly** - declarative RBAC as code

## How It Works

FolderTree uses a "split structure" design that separates concerns:

- **Tree**: Defines the hierarchy (who reports to whom)
- **Folders**: Contains the actual RBAC templates and namespace assignments
- **Controller**: Automatically creates/updates RoleBindings based on inheritance rules

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   FolderTree    │───▶│    Controller    │───▶│  RoleBindings   │
│   Resource      │    │                  │    │  (Auto-created) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         ▲                       │
         │                       ▼
┌─────────────────┐    ┌──────────────────┐
│ Admission       │    │ Event Watchers   │
│ Webhook         │    │ (Drift Detection)│
│ (Validation)    │    └──────────────────┘
└─────────────────┘
```

The controller watches for changes and maintains the desired state, while the admission webhook validates all operations for security.

**Want to understand the architecture?** Read the **[Architecture Deep Dive](PROJECT_SUMMARY.md)**.

## Real-World Examples

### Basic Organizational Hierarchy
Perfect for teams getting started with hierarchical RBAC:
- Platform team gets admin access everywhere
- Environment-specific teams get scoped access
- Application teams get edit access to their services

### Multi-Environment Enterprise
Complex organizational structures with:
- Multiple inheritance levels (org → platform → apps → services)
- Service account permissions for automation
- Emergency access patterns
- Selective propagation for security-sensitive permissions

### GitOps Integration
Declarative RBAC management that works with:
- ArgoCD and Flux deployments
- CI/CD pipeline permissions
- Infrastructure as Code workflows

**See all examples:** **[Demo Examples](demo-examples/README.md)**

## Security & Compliance

### Privilege Escalation Prevention
- **Webhook Validation**: Users can only grant permissions they possess
- **Impersonation Testing**: Dry-run validation with user context
- **Controller Permissions**: Must have all permissions it manages
- **Fail-Safe Design**: Rejects operations if any validation fails

### Production Security
- **TLS Webhooks**: Encrypted communication with cert-manager
- **Network Policies**: Secure inter-component communication
- **Audit Trail**: Comprehensive logging and status reporting
- **RBAC Options**: Minimal or custom permission sets available

**Learn more:** **[Security Model](GUIDE.md#security-model)**

## Community & Support

### 🤝 **Get Help**
- **[Quick Start](QUICKSTART.md)** - Get running in 5 minutes
- **[User Guide](GUIDE.md)** - Comprehensive documentation
- **[Troubleshooting](GUIDE.md#troubleshooting)** - Common issues and solutions
- **[GitHub Issues](https://github.com/mhenriks/kubernetes-foldertree-controller/issues)** - Bug reports and feature requests
- **[Discussions](https://github.com/mhenriks/kubernetes-foldertree-controller/discussions)** - Questions and community support

### 🚀 **Contributing**
We welcome contributions! See our **[Development Guide](GUIDE.md#development)** to get started.

- **Report Issues**: Found a bug? Open an issue with details
- **Feature Requests**: Have an idea? Start a discussion
- **Code Contributions**: Fork, develop, test, and submit PRs
- **Documentation**: Help improve our guides and examples

### 📊 **Project Status**
- **API Version**: v1alpha1 (active development)
- **Production Ready**: Comprehensive testing and security measures
- **License**: Apache 2.0
- **Maintainer**: [@mhenriks](https://github.com/mhenriks)

## Documentation

| Document | Purpose | Audience |
|----------|---------|----------|
| **[Quick Start](QUICKSTART.md)** | Get running in 5 minutes | First-time users |
| **[User Guide](GUIDE.md)** | Comprehensive documentation | Adopters & operators |
| **[Architecture](PROJECT_SUMMARY.md)** | Technical deep dive | Architects & contributors |
| **[Demo Examples](demo-examples/README.md)** | Real-world scenarios | All users |
| **[Presentations](demo-examples/SLIDE_DECK.md)** | Demo materials | Presenters |

## Quick Start Checklist

Ready to transform your RBAC management? Follow these steps:

- [ ] **[Install the controller](QUICKSTART.md#installation-1-minute)** (1 minute)
- [ ] **[Create your first FolderTree](QUICKSTART.md#your-first-foldertree-2-minutes)** (2 minutes)
- [ ] **[Verify inheritance works](QUICKSTART.md#what-just-happened)** (1 minute)
- [ ] **[Explore advanced examples](demo-examples/README.md)** (10 minutes)
- [ ] **[Plan your production deployment](GUIDE.md#production-considerations)** (30 minutes)

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

---

**Ready to eliminate RBAC sprawl?** Start with the **[Quick Start Guide](QUICKSTART.md)** and join the growing community of teams simplifying Kubernetes permissions with FolderTree Controller.

⭐ **Star this repository** if you find it useful!
