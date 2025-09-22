# Controller RBAC Permissions

The FolderTree controller needs permissions to create RoleBindings that reference ClusterRoles. Kubernetes prevents privilege escalation by requiring controllers to have the same permissions they're trying to grant.

## Permission Options

### Option 1: Broad Permissions (Default)
**File**: `controller_permissions.yaml`
**Use when**: You need maximum flexibility and will be using various ClusterRoles like `admin`, `edit`, `view`, etc.
**Security**: Lower - grants broad permissions to the controller

### Option 2: Minimal Permissions
**File**: `controller_permissions_minimal.yaml`
**Use when**: You only need basic permissions equivalent to `view`, `edit`, and `admin` ClusterRoles
**Security**: Higher - grants only common permissions

## Switching Permission Sets

To use minimal permissions instead of broad permissions:

1. Edit `config/rbac/kustomization.yaml`
2. Replace:
   ```yaml
   - controller_permissions.yaml
   ```
   With:
   ```yaml
   - controller_permissions_minimal.yaml
   ```

3. Regenerate and deploy:
   ```bash
   make manifests
   make deploy
   ```

## Custom Permissions

For production environments, consider creating your own permission file with only the exact ClusterRole permissions your FolderTrees will reference.

Example:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foldertree-controller-custom
rules:
# Only the specific permissions your FolderTrees need
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
# Add more as needed...
```

## Troubleshooting

If you see errors like:
```
attempting to grant RBAC permissions not currently held
```

This means the controller doesn't have the permissions it's trying to grant. Either:
1. Use broader controller permissions, or
2. Modify your FolderTree to reference ClusterRoles that match the controller's permissions
