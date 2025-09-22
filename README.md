# Kubernetes Folder Tree Controller

A Kubernetes controller that allows grouping namespaces into a hierarchical structure using a single cluster-scoped resource called `FolderTree` with inline role binding templates. The controller automatically creates and manages `RoleBindings` based on the folder hierarchy and role binding templates.

## Overview

This controller provides:

- **FolderTree**: A single cluster-scoped custom resource that defines hierarchical relationships and folder data with inline RBAC role binding templates
- **Automatic RoleBinding Management**: The controller creates and manages `RoleBindings` in namespaces based on the folder hierarchy
- **Simplified Architecture**: All configuration is contained within the `FolderTree` resource - no additional CRDs needed

## Key Features

- **Hierarchical RBAC**: Organize namespaces into a tree structure with inherited permissions
- **Single Resource Management**: All configuration in one `FolderTree` resource - no additional CRDs needed
- **Secure by Default**: Role binding templates don't inherit unless explicitly enabled with `propagate: true`
- **Automatic Management**: Controller creates and maintains RoleBindings based on your hierarchy
- **Comprehensive Validation**: Admission webhook prevents invalid configurations and privilege escalation
- **Production Ready**: Event-driven architecture with intelligent change detection and status reporting

## Architecture

### Split Structure Design

The `FolderTree` uses a split structure approach to minimize OpenAPI v3 recursive schema validation issues:

#### **TreeNode** (Hierarchy)
- Defines parent-child relationships using names only
- Supports recursive nesting with workaround for CRD schema validation
- References `Folder` objects by name to establish data associations

#### **Folders** (Data)
- Contain permission names and namespace assignments
- Exist as a flat list for easy validation and management
- Can be referenced by the `TreeNode` hierarchy or exist as standalone entities

#### **Role Binding Templates** (Inline RBAC)
- Define RBAC templates directly within each folder with subjects and roleRef
- Eliminate the need for separate `FolderRoleBinding` resources
- Each folder can have multiple role binding templates
- **Selective Propagation**: Use the `propagate` field to control inheritance
  - `propagate: true` - Template inherits to child folders
  - `propagate: false` or unset (default) - Template applies only to current folder

### Controller Logic

The controller processes FolderTrees efficiently:
1. **Mapping**: Links `TreeNode` names to `Folder` data containing RBAC templates and namespaces
2. **Inheritance**: Child folders inherit role binding templates from parents (when `propagate: true`)
3. **Smart Updates**: Only creates/updates/deletes RoleBindings that have actually changed
4. **Event-Driven**: Responds immediately to FolderTree changes, RoleBinding drift, and new namespaces

## Example

```yaml
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: tree1
spec:
  # Define the hierarchical structure
  tree:
    name: root
    subfolders:
    - name: prod
      subfolders:
      - name: prod-app-1
    - name: stage
  # Define the folder data with inline role binding templates
  folders:
  - name: root
    roleBindingTemplates:
    - name: admin
      propagate: true  # Enable inheritance to all child folders
      subjects:
      - kind: Group
        name: operations
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
  - name: prod
    roleBindingTemplates:
    - name: prod-admin
      propagate: true  # Explicitly enable inheritance
      subjects:
      - kind: Group
        name: prod-ops
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    - name: prod-secrets
      # No propagate field - defaults to false (no inheritance)
      subjects:
      - kind: Group
        name: prod-security-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["production-secret"]
  - name: prod-app-1
    roleBindingTemplates:
    - name: prod-admin-app-1
      subjects:
      - kind: Group
        name: prod-ops-app-1
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["production-app-1"]
  - name: stage
    roleBindingTemplates:
    - name: staging-admin
      subjects:
      - kind: Group
        name: stage-admin
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["staging"]
  - name: treeless
    roleBindingTemplates:
    - name: fred
      subjects:
      - kind: User
        name: fred
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["freds-namespace"]
```

In this example:
- **`production-app-1` namespace** gets RoleBindings for: `admin` (from root with `propagate: true`), `prod-admin` (from prod with `propagate: true`), and `prod-admin-app-1` (from prod-app-1)
  - **Does NOT get**: `prod-secrets` (defaults to no inheritance)
- **`production-secret` namespace** gets RoleBindings for: `admin` (from root with `propagate: true`), `prod-admin` (from prod with `propagate: true`), and `prod-secrets` (from prod)
- **`staging` namespace** gets RoleBindings for: `admin` (from root with `propagate: true`) and `staging-admin` (from stage)
- **`freds-namespace` namespace** gets RoleBindings for: `fred` (standalone folder outside tree structure)

Each role binding template (e.g., `admin`, `prod-admin`) is defined inline within the folder and contains the actual RBAC subjects and roleRef.

### Benefits of Split Structure:
- ✅ **Clean separation** of hierarchy vs data
- ✅ **Strict validation** for Folders (data), workaround for TreeNodes (hierarchy)
- ✅ **Flexible design** supports standalone folders
- ✅ **Reduces recursive CRD issues** to minimal scope (only TreeNode.subfolders)

## Installation

### Quick Start (Local Development)

```bash
# Install CRDs
make install

# Run controller locally (webhooks disabled for development)
ENABLE_WEBHOOKS=false make run
```

### Production Deployment

```bash
# Deploy to cluster (includes CRDs, controller, and webhooks)
make deploy
```

The deployment includes:
- Custom Resource Definitions (CRDs)
- Controller with proper RBAC permissions
- Validating admission webhook (requires cert-manager for TLS)

> **Note**: For production, review RBAC permissions in `config/rbac/` and webhook certificates in `config/webhook/`

## Usage

1. Create the required `ClusterRoles` (see `config/samples/test-setup.yaml`)
2. Create a `FolderTree` resource with inline role binding templates (see `config/samples/rbac_v1alpha1_foldertree.yaml`)

The controller will automatically create the appropriate `RoleBindings` in the specified namespaces.

## Validation

The controller includes a validating admission webhook that ensures FolderTree resources are valid and secure before they're created, updated, or deleted in the cluster.

The webhook validates FolderTree resources and prevents common issues:

- **Unique names**: Folder and TreeNode names must be unique
- **Valid references**: TreeNodes must reference existing Folders
- **Namespace conflicts**: Each namespace can only be assigned to one folder
- **RBAC authorization**: Users must have permission to create/modify/delete the RoleBindings
- **Format validation**: Names must follow Kubernetes DNS-1123 format

Invalid resources are rejected with clear error messages explaining what needs to be fixed.

## Security

### Privilege Escalation Prevention

The controller includes important security measures to prevent privilege escalation:

#### **RBAC Authorization Checks**
- **Fine-grained Validation**: The webhook uses **diff analysis + impersonation + dry-run** to test only the specific operations being performed (create/update/delete), validating permissions for actual changes
- **Handles All Permission Types**: Correctly validates clusterroles/clusterrolebindings management permissions AND individual permissions within referenced ClusterRoles
- **Handles Wildcards**: Unlike individual permission checks, this approach correctly handles wildcard permissions (`*`) and aggregated ClusterRoles
- **Shared Logic**: Webhook uses the same RoleBinding creation logic as the controller for accuracy
- **Fail-Safe Design**: If any permission is missing, the dry-run fails and the webhook rejects the FolderTree creation/update

#### **Controller Permissions**

⚠️ **Important**: The controller itself must have all the permissions it manages to avoid "attempting to grant RBAC permissions not currently held" errors.

**The Problem**: Kubernetes prevents controllers from creating RoleBindings that grant permissions the controller doesn't have itself. This is a security feature to prevent privilege escalation.

**The Solution**: The deployment includes `config/rbac/controller_permissions.yaml` which grants the controller broad permissions equivalent to common ClusterRoles like `admin`, `view`, and `edit`.

**Production Considerations**:
- **Review Permissions**: Audit `config/rbac/controller_permissions.yaml` and restrict it to only the permissions your FolderTrees actually need
- **Alternative Permission Sets**: See `config/rbac/README.md` for minimal permission alternatives and customization guidance
- **Principle of Least Privilege**: Consider creating custom ClusterRoles with minimal permissions instead of using broad roles like `admin`
- **Regular Audits**: Regularly review what permissions the controller has and what FolderTrees are granting

#### **How Security Prevention Works:**

The webhook prevents privilege escalation through two main checks:

**1. RoleBinding Management Check**
- User must have permission to create/update/delete RoleBindings in the target namespaces
- Without this, the user can't perform the basic RoleBinding operations the controller needs

**2. Individual Permission Check**
- User must have all the individual permissions contained within the ClusterRoles they're trying to grant
- This prevents users from granting permissions they don't have themselves
- Works correctly with wildcard permissions (`*`) and aggregated ClusterRoles

**Common Rejection Scenarios:**
- User tries to create RoleBindings but lacks `rolebindings.rbac.authorization.k8s.io` permissions
- User has RoleBinding permissions but lacks specific permissions like "create pods" that are in the ClusterRole they're referencing
- User tries to delete a FolderTree but lacks permission to delete the existing RoleBindings

The webhook shows clear error messages explaining exactly which permission check failed.

#### **Required User Permissions:**
To create/update/delete FolderTrees, users need permissions for **only the specific RoleBinding operations** the controller will perform. The webhook validates each operation before allowing the FolderTree change.

**Examples of what permissions you need:**

- **Adding a new role binding template** → Permission to CREATE RoleBindings with those specific permissions
- **Removing a role binding template** → Permission to DELETE the existing RoleBindings
- **Changing template subjects or roleRef** → Permission to UPDATE the RoleBindings
- **Adding a namespace to a folder** → Permission to CREATE RoleBindings in that namespace
- **Deleting a FolderTree** → Permission to DELETE all RoleBindings that would be removed

The webhook shows exactly which RoleBinding operation failed if you lack the required permissions.

## Development

### Prerequisites

- Go 1.21+
- kubectl
- Kubernetes cluster (kind/minikube for local testing)

### Building

```bash
# Generate manifests and build
make generate && make manifests
make build

# Run tests
make test

# Build and push Docker image
make docker-build docker-push IMG=<registry>/folders:tag
```

### Project Structure

```
├── api/v1alpha1/           # CRD definitions
├── internal/controller/    # Controller reconciliation logic
├── internal/rbac/         # RBAC calculation and diff analysis
├── internal/webhook/      # Admission webhook validation
├── config/                # Kubernetes manifests
│   ├── samples/          # Example FolderTree resources
│   └── rbac/             # Controller permissions
└── demo-examples/         # Demo and testing resources
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0.
