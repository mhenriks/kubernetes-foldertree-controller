# FolderTree Controller - Complete User Guide

Comprehensive documentation for adopting and using the FolderTree Controller in production environments.

## Table of Contents

- [How It Works](#how-it-works)
- [Architecture](#architecture)
- [Usage Examples](#usage-examples)
- [Security Model](#security-model)
- [Installation & Deployment](#installation--deployment)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)
- [Production Considerations](#production-considerations)
- [Development](#development)
- [Migration Guide](#migration-guide)

## How It Works

### Core Concepts

FolderTree Controller transforms RBAC management through three key concepts:

1. **Hierarchical Organization**: Namespaces organized into tree structures that mirror your organization
2. **Selective Inheritance**: Fine-grained control over which permissions flow down the hierarchy
3. **Declarative Management**: Single resource defines entire RBAC structure

### Split Structure Design

The controller uses an innovative "split structure" to overcome Kubernetes OpenAPI v3 recursive schema limitations:

```yaml
spec:
  tree:           # Hierarchy Definition
    name: root
    subfolders:
    - name: production
      subfolders:
      - name: web-app

  folders:        # Data Definition
  - name: root
    roleBindingTemplates: [...]  # Inline RBAC templates
    namespaces: [...]           # Namespace assignments
```

**Benefits:**
- ✅ Clean separation of hierarchy vs data
- ✅ Overcomes OpenAPI v3 recursive limitations
- ✅ Supports standalone folders outside tree structures
- ✅ Enables strict validation for all components

### Inheritance Rules

```yaml
roleBindingTemplates:
- name: platform-admin
  propagate: true    # ✅ Inherits to ALL child folders
- name: secrets-access
  propagate: false   # ❌ Applies ONLY to current folder (default)
- name: monitoring
  # No propagate field = defaults to false (secure by default)
```

**Inheritance Flow:**
```
root (admin: propagate=true)
├── production (prod-ops: propagate=true, secrets: propagate=false)
│   ├── web-app → Gets: admin + prod-ops (NOT secrets)
│   └── api-service → Gets: admin + prod-ops (NOT secrets)
└── staging → Gets: admin only
```

## Architecture

### Component Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   FolderTree    │───▶│    Controller    │───▶│  RoleBindings   │
│   Resource      │    │   (Reconciler)   │    │  (Auto-managed) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         ▲                       │                       │
         │                       ▼                       │
┌─────────────────┐    ┌──────────────────┐              │
│ Admission       │    │ Event Watchers   │              │
│ Webhook         │    │ • FolderTree     │              │
│ (Validation)    │    │ • RoleBinding    │              │
└─────────────────┘    │ • Namespace      │              │
                       └──────────────────┘              │
                                │                        │
                                ▼                        │
                       ┌──────────────────┐              │
                       │ RBAC Engine      │◀─────────────┘
                       │ • Calculation    │
                       │ • Diff Analysis  │
                       │ • Inheritance    │
                       └──────────────────┘
```

### Controller Logic

1. **Event-Driven**: Watches FolderTree, RoleBinding, and Namespace resources
2. **Smart Diff Analysis**: Only updates what actually changed
3. **Inheritance Processing**: Calculates effective permissions for each namespace
4. **Reconciliation**: Creates/updates/deletes RoleBindings to match desired state

### Namespace Handling

The controller has intelligent handling for namespace lifecycle events:

#### Deleted Namespaces

**Controller Behavior:**
- If a namespace referenced in a FolderTree is deleted, the controller **silently skips** creating RoleBindings in that namespace
- When the namespace is recreated, the controller automatically creates the appropriate RoleBindings
- RoleBindings are automatically cleaned up by Kubernetes garbage collection when namespaces are deleted

**Webhook Validation:**
- **New namespaces** (added to FolderTree): **MUST exist** - validation fails if namespace doesn't exist
- **Existing namespaces** (already in FolderTree): **Can be deleted** - validation succeeds even if namespace was deleted
- This allows FolderTrees to be updated or deleted even when some namespaces have been removed

**Example Workflow:**
```bash
# 1. Create FolderTree with namespace "prod-web"
kubectl apply -f foldertree.yaml

# 2. Delete the namespace
kubectl delete namespace prod-web

# 3. Update the FolderTree (e.g., add new permissions)
kubectl apply -f foldertree.yaml  # ✅ Succeeds - "prod-web" was already in tree

# 4. Delete the FolderTree
kubectl delete foldertree my-tree  # ✅ Succeeds - validates only existing namespaces

# 5. Try to add a NEW non-existent namespace
# Edit foldertree.yaml to add "new-namespace" that doesn't exist
kubectl apply -f foldertree.yaml  # ❌ Fails - new namespaces must exist
```

**Why This Design?**
- **Operational Flexibility**: Allows namespace lifecycle management independent of FolderTrees
- **No Lock-in**: Can delete FolderTrees even when namespaces are already gone
- **Safety**: Prevents accidentally referencing non-existent namespaces when adding new ones
- **Event-Driven Recovery**: Automatically reconciles when namespaces are recreated

### Admission Webhook

- **Validation**: Comprehensive business logic and security checks
- **Privilege Escalation Prevention**: Users can only grant permissions they possess
- **Real-time Feedback**: Clear error messages for invalid configurations

## Usage Examples

### Basic Organizational Hierarchy

```yaml
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: basic-org
spec:
  tree:
    name: root
    subfolders:
    - name: production
      subfolders:
      - name: web-services
      - name: data-services
    - name: staging
    - name: development

  folders:
  - name: root
    roleBindingTemplates:
    - name: platform-admin
      propagate: true
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
    - name: prod-operators
      propagate: true
      subjects:
      - kind: Group
        name: production-ops
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    - name: prod-secrets
      # No propagate - restricted to production level only
      subjects:
      - kind: Group
        name: security-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["prod-shared", "prod-monitoring"]

  - name: web-services
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
    namespaces: ["prod-web", "prod-frontend"]

  - name: data-services
    roleBindingTemplates:
    - name: data-engineers
      subjects:
      - kind: Group
        name: data-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["prod-database", "prod-analytics"]

  - name: staging
    roleBindingTemplates:
    - name: all-developers
      subjects:
      - kind: Group
        name: developers
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["staging"]

  - name: development
    roleBindingTemplates:
    - name: dev-users
      subjects:
      - kind: Group
        name: developers
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["dev-playground", "dev-experiments"]
```

### Multi-Environment with Service Accounts

```yaml
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: multi-env-org
spec:
  tree:
    name: organization
    subfolders:
    - name: platform
      subfolders:
      - name: applications
        subfolders:
        - name: ecommerce
          subfolders:
          - name: frontend
          - name: backend
    - name: automation

  folders:
  - name: organization
    roleBindingTemplates:
    - name: org-admins
      propagate: true
      subjects:
      - kind: Group
        name: organization-admins
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    - name: org-viewers
      propagate: true
      subjects:
      - kind: Group
        name: all-employees
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io

  - name: platform
    roleBindingTemplates:
    - name: platform-operators
      propagate: true
      subjects:
      - kind: Group
        name: platform-engineering
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["kube-system", "monitoring", "logging"]

  - name: applications
    roleBindingTemplates:
    - name: app-architects
      propagate: true
      subjects:
      - kind: Group
        name: application-architects
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io

  - name: ecommerce
    roleBindingTemplates:
    - name: ecommerce-leads
      propagate: true
      subjects:
      - kind: Group
        name: ecommerce-leadership
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io

  - name: frontend
    roleBindingTemplates:
    - name: frontend-developers
      subjects:
      - kind: Group
        name: frontend-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ecommerce-frontend-prod", "ecommerce-frontend-stage"]

  - name: backend
    roleBindingTemplates:
    - name: backend-developers
      subjects:
      - kind: Group
        name: backend-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ecommerce-backend-prod", "ecommerce-backend-stage"]

  - name: automation
    roleBindingTemplates:
    - name: ci-cd-access
      subjects:
      - kind: ServiceAccount
        name: argocd-server
        namespace: argocd
      - kind: ServiceAccount
        name: github-actions
        namespace: ci-cd
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["argocd", "ci-cd", "automation-tools"]
```

### Standalone Folders (Outside Tree)

```yaml
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: mixed-structure
spec:
  tree:
    name: main-org
    subfolders:
    - name: production

  folders:
  - name: main-org
    roleBindingTemplates:
    - name: org-admin
      propagate: true
      subjects:
      - kind: Group
        name: admins
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io

  - name: production
    namespaces: ["prod-main"]

  # Standalone folders - not part of tree hierarchy
  - name: sandbox
    roleBindingTemplates:
    - name: sandbox-users
      subjects:
      - kind: Group
        name: developers
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["sandbox", "experiments"]

  - name: external-project
    roleBindingTemplates:
    - name: external-team
      subjects:
      - kind: Group
        name: contractors
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["external-work"]
```

## Security Model

### Privilege Escalation Prevention

The controller implements comprehensive security measures:

#### 1. RBAC Authorization Checks

**How it works:**
- Webhook uses **diff analysis + impersonation + dry-run** to validate operations
- Tests only specific operations being performed (create/update/delete)
- Validates both RoleBinding management permissions AND individual permissions

**What gets validated:**
```bash
# For each RoleBinding operation, user must have:
1. Permission to manage RoleBindings in target namespace
2. All individual permissions contained in the referenced ClusterRole
```

**Example validation:**
```yaml
# User wants to create this template:
roleBindingTemplates:
- name: web-admin
  subjects:
  - kind: Group
    name: web-team
  roleRef:
    kind: ClusterRole
    name: admin  # Contains pods/*, services/*, etc.

# Webhook validates:
# ✅ User can create RoleBindings in target namespaces?
# ✅ User has pods/* permissions?
# ✅ User has services/* permissions?
# ✅ User has all other permissions in 'admin' ClusterRole?
```

#### 2. Controller Permissions

**The Challenge:** Kubernetes prevents controllers from creating RoleBindings that grant permissions the controller doesn't have itself.

**Solutions Available:**

**Option 1: Broad Permissions (Default)**
```yaml
# config/rbac/controller_permissions.yaml
# Grants controller extensive permissions equivalent to admin/edit/view ClusterRoles
```

**Option 2: Minimal Permissions**
```yaml
# config/rbac/controller_permissions_minimal.yaml
# Grants only common permissions for basic use cases
```

**Option 3: Custom Permissions**
```yaml
# Create your own ClusterRole with exactly the permissions your FolderTrees need
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foldertree-controller-custom
rules:
- apiGroups: [""]
  resources: ["pods", "services", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
# Add only what you actually use
```

#### 3. Validation Features

**Structural Validation:**
- Unique names across folders and tree nodes
- Valid DNS-1123 naming conventions
- Proper cross-references between tree nodes and folders
- Namespace assignment conflicts prevention

**Business Logic Validation:**
- Inheritance conflict detection
- Reasonable resource limits (folders, namespaces, templates)
- Required field validation
- Circular reference prevention

**Security Validation:**
- User permission verification for all operations
- Privilege escalation prevention
- Dry-run validation with user impersonation
- Clear error messages for failed validations

### Required User Permissions

Users need permissions for **only the specific operations** the controller will perform:

| Operation | Required Permission |
|-----------|-------------------|
| Add role binding template | CREATE RoleBindings + all permissions in referenced ClusterRole |
| Remove role binding template | DELETE existing RoleBindings |
| Change template subjects/roleRef | UPDATE RoleBindings + new permissions |
| Add namespace to folder | CREATE RoleBindings in that namespace |
| Delete FolderTree | DELETE all RoleBindings that would be removed |

**Permission Check Examples:**
```bash
# Check if you can create RoleBindings
kubectl auth can-i create rolebindings --namespace=prod-web

# Check if you have specific permissions you're trying to grant
kubectl auth can-i create pods --namespace=prod-web
kubectl auth can-i get services --namespace=prod-web

# Check wildcard permissions
kubectl auth can-i '*' '*' --namespace=prod-web
```

## Installation & Deployment

### Development Setup

```bash
# Clone repository
git clone https://github.com/mhenriks/kubernetes-foldertree-controller
cd kubernetes-foldertree-controller

# Install dependencies
make install

# Run locally (webhooks disabled)
ENABLE_WEBHOOKS=false make run
```

### Production Deployment

#### Prerequisites
```bash
# Ensure cert-manager is installed (required for webhook TLS)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Verify cert-manager is ready
kubectl wait --for=condition=ready pod -l app=cert-manager -n cert-manager --timeout=60s
```

#### Deploy Controller
```bash
# Deploy everything (CRDs, controller, webhooks, RBAC)
make deploy

# Verify deployment
kubectl get pods -n foldertree-system
kubectl get crd foldertrees.rbac.kubevirt.io
kubectl get validatingwebhookconfigurations
```

#### Custom Image
```bash
# Build and push custom image
make docker-build docker-push IMG=your-registry/folders:v1.0.0

# Deploy with custom image
make deploy IMG=your-registry/folders:v1.0.0
```

### Configuration Options

#### Environment Variables
```yaml
# In deployment
env:
- name: ENABLE_WEBHOOKS
  value: "true"  # Set to false for development
- name: METRICS_BIND_ADDRESS
  value: ":8080"
- name: HEALTH_PROBE_BIND_ADDRESS
  value: ":8081"
```

#### Controller Permissions
```bash
# Switch to minimal permissions
# Edit config/rbac/kustomization.yaml:
resources:
- controller_permissions_minimal.yaml  # Instead of controller_permissions.yaml

# Regenerate and deploy
make manifests && make deploy
```

#### Webhook Configuration
```yaml
# config/webhook/manifests.yaml
webhooks:
- name: foldertree.rbac.kubevirt.io
  failurePolicy: Fail  # Change to Ignore for non-critical environments
  sideEffects: None
```

## Troubleshooting

### Common Issues and Solutions

#### Controller Issues

**"attempting to grant RBAC permissions not currently held"**
```bash
# Problem: Controller lacks permissions it's trying to grant
# Solution 1: Use broader controller permissions
kubectl apply -f config/rbac/controller_permissions.yaml

# Solution 2: Grant specific permissions to controller
kubectl create clusterrole foldertree-custom --verb=get,list,create,update,delete --resource=pods,services
kubectl create clusterrolebinding foldertree-custom --clusterrole=foldertree-custom --serviceaccount=foldertree-system:foldertree-controller-manager
```

**Controller not starting**
```bash
# Check CRDs are installed
kubectl get crd foldertrees.rbac.kubevirt.io

# Check RBAC permissions
kubectl get clusterrole foldertree-controller-manager-role
kubectl get clusterrolebinding foldertree-controller-manager-rolebinding

# Check controller logs
kubectl logs -n foldertree-system deployment/foldertree-controller-manager
```

**RoleBindings not appearing**
```bash
# Verify namespaces exist
kubectl get namespace <namespace-name>

# Check FolderTree status
kubectl get foldertree <name> -o yaml | grep -A 10 status

# Check controller reconciliation
kubectl logs -f -n foldertree-system deployment/foldertree-controller-manager | grep "Reconciling FolderTree"
```

#### Webhook Issues

**Webhook validation failures**
```bash
# Check webhook is running
kubectl get pods -n foldertree-system | grep webhook

# Check webhook configuration
kubectl get validatingwebhookconfigurations

# Check certificates
kubectl get certificates -n foldertree-system
kubectl describe certificate webhook-cert -n foldertree-system
```

**Certificate issues**
```bash
# Verify cert-manager is working
kubectl get pods -n cert-manager

# Check certificate status
kubectl get certificates -A
kubectl describe certificate <cert-name> -n foldertree-system

# Force certificate renewal
kubectl delete certificate webhook-cert -n foldertree-system
# Controller will recreate it automatically
```

**Permission denied during validation**
```bash
# Check your permissions for the operation you're trying
kubectl auth can-i create rolebindings --namespace=<target-namespace>

# For development, you might need cluster-admin
kubectl create clusterrolebinding my-admin --clusterrole=cluster-admin --user=$(kubectl config current-context)
```

#### FolderTree Issues

**Validation errors**
```bash
# Common validation failures and fixes:

# 1. Duplicate names
# Error: "folder name 'prod' already used"
# Fix: Ensure all folder names are unique within the FolderTree

# 2. Invalid references
# Error: "tree node 'web-app' references undeclared folder"
# Fix: Ensure all tree node names have corresponding folders

# 3. Namespace conflicts
# Error: "namespace 'prod-web' is already assigned"
# Fix: Each namespace can only belong to one folder

# 4. DNS-1123 naming
# Error: "name must be a valid DNS-1123 label"
# Fix: Use lowercase alphanumeric characters and hyphens only

# 5. Non-existent namespace (NEW in this update)
# Error: "namespace 'my-ns' does not exist - cannot add non-existent namespace"
# Fix: This only happens when ADDING a new namespace. Create it first:
kubectl create namespace my-ns
kubectl apply -f foldertree.yaml  # Now it works

# NOTE: Existing namespaces CAN be deleted - updates will still work
```

**Namespace-related issues**
```bash
# Problem: "namespace does not exist" error when creating/updating FolderTree
# This is expected behavior - NEW namespaces must exist before adding to FolderTree

# Solution: Create the namespace first
kubectl create namespace <new-namespace>
kubectl apply -f foldertree.yaml

# Problem: Cannot delete FolderTree after namespace was deleted
# This should NOT happen with current controller - it handles deleted namespaces gracefully

# Problem: RoleBindings not appearing in newly created namespace
# The controller watches for namespace creation events - should appear within seconds

# Verify namespace was created
kubectl get namespace <namespace-name>

# Manually trigger reconciliation
kubectl annotate foldertree <name> reconcile="$(date +%s)"

# Check controller logs
kubectl logs -f -n foldertree-system deployment/foldertree-controller-manager | grep "namespace"

# Problem: Want to update FolderTree after namespace was deleted
# This is SUPPORTED - existing namespaces can be deleted

# The controller will skip creating RoleBindings in deleted namespaces
kubectl apply -f foldertree.yaml  # Works fine

# Check logs - should show "Skipping validation for non-existent namespace"
kubectl logs -n foldertree-system deployment/foldertree-controller-manager | grep "Skipping"

# When namespace is recreated, RoleBindings automatically appear
kubectl create namespace <namespace-name>
# Wait a few seconds, then check:
kubectl get rolebindings -n <namespace-name>
```

### Debug Commands

```bash
# Monitor controller reconciliation
kubectl logs -f -n foldertree-system deployment/foldertree-controller-manager

# Check FolderTree events
kubectl describe foldertree <name>

# List all managed RoleBindings
kubectl get rolebindings -A -l foldertree.rbac.kubevirt.io/tree=<foldertree-name>

# Check webhook logs
kubectl logs -n foldertree-system deployment/foldertree-controller-manager | grep webhook

# Validate webhook configuration
kubectl get validatingwebhookconfigurations -o yaml

# Test webhook directly
kubectl apply --dry-run=server -f your-foldertree.yaml
```

### Performance Troubleshooting

```bash
# Check controller metrics
kubectl port-forward -n foldertree-system svc/foldertree-controller-manager-metrics-service 8080:8443
curl -k https://localhost:8080/metrics

# Monitor resource usage
kubectl top pods -n foldertree-system

# Check for resource limits
kubectl describe pod -n foldertree-system -l control-plane=controller-manager
```

## Production Considerations

### Scalability

**Resource Limits:**
- Max 100 folders per FolderTree
- Max 100 tree nodes per FolderTree
- Max 500 namespace assignments total
- Max 200 role binding templates total

**Performance Characteristics:**
- Event-driven architecture (no polling)
- Intelligent diff analysis (only updates changes)
- Efficient inheritance calculation
- Minimal API server load

### Monitoring & Observability

**Health Checks:**
```bash
# Controller health endpoints
curl http://controller:8081/healthz
curl http://controller:8081/readyz
```

**Metrics:**
```bash
# Prometheus metrics available at :8080/metrics
# Key metrics:
# - controller_runtime_reconcile_total
# - controller_runtime_reconcile_errors_total
# - workqueue_depth
# - rest_client_requests_total
```

**Logging:**
```yaml
# Configure log levels
args:
- --zap-log-level=info  # debug, info, error
- --zap-development=false
```

### Backup & Recovery

**FolderTree Backup:**
```bash
# Export all FolderTrees
kubectl get foldertrees -o yaml > foldertrees-backup.yaml

# Restore
kubectl apply -f foldertrees-backup.yaml
```

**RoleBinding Recovery:**
```bash
# If RoleBindings are accidentally deleted, they'll be recreated automatically
# Force reconciliation:
kubectl annotate foldertree <name> kubectl.kubernetes.io/last-applied-configuration-

# Or restart controller:
kubectl rollout restart deployment/foldertree-controller-manager -n foldertree-system
```

### High Availability

**Controller HA:**
```yaml
# Increase replicas with leader election
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=true
        - --leader-election-id=foldertree-controller
```

**Webhook HA:**
```yaml
# Multiple webhook replicas
spec:
  replicas: 2
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  control-plane: controller-manager
              topologyKey: kubernetes.io/hostname
```

### Security Hardening

**Network Policies:**
```yaml
# Restrict controller network access
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: folder-controller-netpol
  namespace: foldertree-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 9443  # Webhook port
  egress:
  - to: []  # Kubernetes API server
    ports:
    - protocol: TCP
      port: 443
```

**Pod Security Standards:**
```yaml
# Apply restricted pod security
apiVersion: v1
kind: Namespace
metadata:
  name: foldertree-system
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

## Development

### Prerequisites
- Go 1.24+
- kubectl
- Kubernetes cluster (kind/minikube/cloud)
- Docker (for building images)

### Development Workflow

```bash
# Set up development environment
make install                    # Install CRDs
ENABLE_WEBHOOKS=false make run  # Run controller locally

# In another terminal, test changes
kubectl apply -f config/samples/rbac_v1alpha1_foldertree.yaml
kubectl get rolebindings -A | grep foldertree

# Run tests
make test                       # Unit tests
make test-e2e                   # End-to-end tests (requires Kind)

# Code generation
make generate                   # Generate deepcopy methods
make manifests                  # Generate CRDs and RBAC
```

### Testing

**Unit Tests:**
```bash
# Run all unit tests
make test

# Run specific test
go test ./internal/controller -v -run TestFolderTreeReconciler

# Run with coverage
make test ARGS="-coverprofile=coverage.out"
go tool cover -html=coverage.out
```

**Integration Tests:**
```bash
# Uses envtest (real Kubernetes API server)
go test ./internal/controller -v -tags=integration
```

**End-to-End Tests:**
```bash
# Requires Kind cluster
make test-e2e

# Manual e2e testing
kind create cluster --name foldertree-test
make deploy
kubectl apply -f demo-examples/basic-hierarchy.yaml
```

### Contributing

**Code Style:**
```bash
# Format code
make fmt

# Lint code
make vet

# Generate code
make generate
```

**Pull Request Process:**
1. Fork repository
2. Create feature branch
3. Make changes with tests
4. Run full test suite
5. Update documentation
6. Submit pull request

**Project Structure:**
```
├── api/v1alpha1/           # CRD definitions and types
├── cmd/main.go            # Application entry point
├── internal/
│   ├── controller/        # Reconciliation logic
│   ├── rbac/             # RBAC calculation engine
│   └── webhook/          # Admission webhook
├── config/               # Kubernetes manifests
├── demo-examples/        # Demo scenarios
└── test/                # Test suites
```

## Migration Guide

### From Manual RoleBindings

**Step 1: Audit Current State**
```bash
# List all existing RoleBindings
kubectl get rolebindings -A -o yaml > current-rolebindings.yaml

# Analyze patterns
grep -E "subjects|roleRef" current-rolebindings.yaml | sort | uniq -c
```

**Step 2: Design FolderTree Structure**
```yaml
# Map your current structure to FolderTree
# Example mapping:
# prod-web-admin-binding → web-app folder with admin template
# prod-api-admin-binding → api-service folder with admin template
# stage-dev-binding → staging folder with edit template
```

**Step 3: Gradual Migration**
```bash
# 1. Deploy controller
make deploy

# 2. Create FolderTree for subset of namespaces
kubectl apply -f migration-phase1.yaml

# 3. Verify RoleBindings are created correctly
kubectl get rolebindings -A | grep foldertree

# 4. Remove old manual RoleBindings
kubectl delete rolebinding old-manual-binding -n target-namespace

# 5. Repeat for remaining namespaces
```

**Step 4: Validation**
```bash
# Verify permissions work as expected
kubectl auth can-i get pods --as=group:web-team --namespace=prod-web
kubectl auth can-i create services --as=group:platform-team --namespace=prod-web
```

### From Other RBAC Tools

**From Helm Charts:**
```bash
# Export existing RBAC resources
helm template my-app ./chart | grep -A 20 "kind: RoleBinding" > existing-rbac.yaml

# Convert to FolderTree structure
# Map chart values to FolderTree folders and templates
```

**From Kustomize:**
```bash
# Build current kustomization
kustomize build . | grep -A 20 "kind: RoleBinding" > current-rbac.yaml

# Design equivalent FolderTree
# Replace kustomize RBAC patches with FolderTree inheritance
```

### Rollback Strategy

**Preparation:**
```bash
# Before migration, export current RoleBindings
kubectl get rolebindings -A -o yaml > rollback-rolebindings.yaml

# Create rollback script
cat > rollback.sh << 'EOF'
#!/bin/bash
kubectl delete foldertree --all
kubectl apply -f rollback-rolebindings.yaml
EOF
chmod +x rollback.sh
```

**Emergency Rollback:**
```bash
# Quick rollback if issues occur
./rollback.sh

# Or manual rollback
kubectl delete foldertree my-org
kubectl apply -f rollback-rolebindings.yaml
```

---

## Next Steps

- **[Quick Start Guide](QUICKSTART.md)** - Get running in 5 minutes
- **[Architecture Deep Dive](PROJECT_SUMMARY.md)** - Technical implementation details
- **[Demo Examples](demo-examples/README.md)** - Real-world scenarios and presentations
- **[GitHub Issues](https://github.com/mhenriks/kubernetes-foldertree-controller/issues)** - Report bugs or request features
- **[Discussions](https://github.com/mhenriks/kubernetes-foldertree-controller/discussions)** - Ask questions and share experiences
