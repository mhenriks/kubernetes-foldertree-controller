# Recursive CRD Schema Fix

This document explains the recursive schema issue in the FolderTree CRD and how it's automatically fixed during the build process.

## The Problem

The `FolderTree` CRD contains a recursive type definition in the `TreeNode` structure:

```go
type TreeNode struct {
    Name       string     `json:"name"`
    Subfolders []TreeNode `json:"subfolders,omitempty"` // Recursive!
}
```

When kubebuilder's `controller-gen` generates the OpenAPI v3 schema, it cannot properly handle recursive types and generates:

```yaml
subfolders:
  type: array
  items: {} # Invalid! Must have a type
```

This causes Kubernetes to reject the CRD with:
```
spec.validation.openAPIV3Schema.properties[...].subfolders.items.type: Required value: must not be empty for specified array items
```

## The Solution

We use a **post-processing script** that automatically fixes the generated CRD:

### 1. Integrated Build Process

```makefile
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	@python3 hack/fix-recursive-crd.py
```

### 2. Automatic Fix Applied

The script transforms:
```yaml
# BEFORE (broken)
subfolders:
  description: Subfolders is a list of child folders
  # Missing items schema!
```

Into:
```yaml
# AFTER (fixed)
subfolders:
  description: Subfolders is a list of child folders
  type: array
  items:
    type: object
    x-kubernetes-preserve-unknown-fields: true
```

## Benefits

✅ **Fully Automated**: No manual intervention required
✅ **Build Integration**: Works with all make targets (`test`, `build`, `deploy`)
✅ **Version Control Friendly**: Generated files are consistent
✅ **Maintainable**: Single script handles the fix logic

## Alternative Approaches

### 1. Kustomize Patches
- **Pros**: Declarative, version-controlled patches
- **Cons**: More complex setup, requires kustomize knowledge

### 2. Custom Controller-Gen Tool
- **Pros**: Native integration with kubebuilder workflow
- **Cons**: Requires maintaining custom tooling

### 3. Manual Schema Definition
- **Pros**: Full control over schema
- **Cons**: Loses code-first benefits of kubebuilder

## Implementation Details

The fix script (`hack/fix-recursive-crd.py`):
1. Loads the generated CRD YAML
2. Navigates to the problematic `subfolders` field
3. Replaces empty `items: {}` with proper object schema
4. Uses `x-kubernetes-preserve-unknown-fields: true` to allow recursive content
5. Writes the fixed CRD back to disk

## Testing

The fix is automatically tested in the build process:
```bash
make test  # Includes CRD validation in test environment
```

## Maintenance

- **Script Location**: `hack/fix-recursive-crd.py`
- **Target CRD**: `config/crd/bases/rbac.kubevirt.io_foldertrees.yaml`
- **Integration**: `Makefile` `manifests` target

If the CRD structure changes significantly, the script may need updates to navigate the YAML structure correctly.
