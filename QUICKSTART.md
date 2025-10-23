# Quick Start Guide

Get up and running with FolderTree Controller in under 5 minutes.

## Prerequisites (30 seconds)

- Kubernetes cluster (kind/minikube/cloud)
- kubectl configured
- Cluster admin permissions

```bash
# Verify your setup
kubectl cluster-info
kubectl auth can-i '*' '*' --all-namespaces
```

## Installation (1 minute)

### Option 1: Development (Local)
```bash
# Clone and install
git clone https://github.com/mhenriks/kubernetes-foldertree-controller
cd kubernetes-foldertree-controller

# Install CRDs and run locally
make install
ENABLE_WEBHOOKS=false make run
```

### Option 2: Production (Cluster)
```bash
# Deploy everything to cluster
make deploy

# Verify installation
kubectl get pods -n foldertree-system
kubectl get crd foldertrees.rbac.kubevirt.io
```

## Your First FolderTree (2 minutes)

### Step 1: Create Required Namespaces
```bash
kubectl create namespace prod-web
kubectl create namespace prod-api
kubectl create namespace staging
```

### Step 2: Apply Your First FolderTree
```bash
kubectl apply -f - <<EOF
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
    - name: staging
  folders:
  - name: platform
    roleBindingTemplates:
    - name: platform-admin
      propagate: true  # This will inherit to ALL child folders
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
      propagate: true  # This inherits to production children only
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
      # No propagate = only applies to this folder
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
  - name: staging
    roleBindingTemplates:
    - name: staging-users
      subjects:
      - kind: Group
        name: all-developers
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["staging"]
EOF
```

### Step 3: Watch the Magic Happen
```bash
# See all the RoleBindings that were automatically created
kubectl get rolebindings -A | grep foldertree

# Check specific namespace inheritance
echo "=== prod-web namespace gets ==="
kubectl get rolebindings -n prod-web
echo ""
echo "=== prod-api namespace gets ==="
kubectl get rolebindings -n prod-api
echo ""
echo "=== staging namespace gets ==="
kubectl get rolebindings -n staging
```

## What Just Happened?

🎉 **You just replaced 7+ manual RoleBindings with 1 simple FolderTree!**

Here's what the controller automatically created:

### `prod-web` namespace received 3 RoleBindings:
- ✅ `platform-admin` (inherited from platform)
- ✅ `prod-ops` (inherited from production)
- ✅ `web-developers` (direct from web-app folder)

### `prod-api` namespace received 3 RoleBindings:
- ✅ `platform-admin` (inherited from platform)
- ✅ `prod-ops` (inherited from production)
- ✅ `api-developers` (direct from api-service folder)

### `staging` namespace received 2 RoleBindings:
- ✅ `platform-admin` (inherited from platform)
- ✅ `staging-users` (direct from staging folder)

### The Inheritance Flow:
```
platform (platform-admin propagates ✓)
├── production (prod-ops propagates ✓)
│   ├── web-app → prod-web namespace
│   └── api-service → prod-api namespace
└── staging → staging namespace
```

## Test Your Setup

### Verify Inheritance Works
```bash
# Add a new namespace to an existing folder
kubectl patch foldertree my-organization --type='merge' -p='
spec:
  folders:
  - name: web-app
    namespaces: ["prod-web", "prod-web-2"]  # Add second namespace
    roleBindingTemplates:
    - name: web-developers
      subjects:
      - kind: Group
        name: web-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: edit
        apiGroup: rbac.authorization.k8s.io'

# Create the new namespace
kubectl create namespace prod-web-2

# Watch RoleBindings appear automatically
kubectl get rolebindings -n prod-web-2
```

### Test Selective Propagation
```bash
# Add a secrets-only permission that doesn't propagate
kubectl patch foldertree my-organization --type='merge' -p='
spec:
  folders:
  - name: production
    roleBindingTemplates:
    - name: prod-ops
      propagate: true
      subjects:
      - kind: Group
        name: production-operators
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    - name: secrets-access
      # No propagate = stays only in production namespaces
      subjects:
      - kind: Group
        name: security-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io'

# Check that secrets-access does NOT appear in child namespaces
kubectl get rolebindings -n prod-web | grep secrets-access  # Should be empty
kubectl get rolebindings -n prod-api | grep secrets-access  # Should be empty
```

## Common First-Time Issues

### "attempting to grant RBAC permissions not currently held"
```bash
# The controller needs the permissions it's granting
# For development, this usually means you need cluster-admin
kubectl create clusterrolebinding my-admin --clusterrole=cluster-admin --user=$(kubectl config current-context)
```

### Webhook validation errors (production deployments)
```bash
# Check if cert-manager is installed
kubectl get pods -n cert-manager

# If not installed, install cert-manager first
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
```

### RoleBindings not appearing
```bash
# Check controller logs
kubectl logs -f -n foldertree-system deployment/foldertree-controller-manager

# Verify FolderTree status
kubectl get foldertree my-organization -o yaml | grep -A 10 status
```

## Next Steps

### 🎯 **Explore More Examples**
```bash
# Try the comprehensive demo scenarios
kubectl apply -f demo-examples/basic-hierarchy.yaml
kubectl apply -f demo-examples/advanced-scenarios.yaml
```

### 📚 **Learn the Concepts**
- **[Complete User Guide](GUIDE.md)** - Comprehensive documentation
- **[Architecture Deep Dive](PROJECT_SUMMARY.md)** - How it all works
- **[Demo Examples](demo-examples/README.md)** - Real-world scenarios

### 🛡️ **Production Planning**
- **Security Model** - Understanding privilege escalation prevention
- **RBAC Configuration** - Controller permissions and customization
- **Monitoring & Observability** - Health checks and metrics

### 🤝 **Get Help**
- **Issues**: Found a bug? [Open an issue](https://github.com/mhenriks/kubernetes-foldertree-controller/issues)
- **Discussions**: Questions? Start a [discussion](https://github.com/mhenriks/kubernetes-foldertree-controller/discussions)
- **Contributing**: Want to help? See the [development guide](GUIDE.md#development)

## Clean Up (Optional)

```bash
# Remove the test FolderTree (this will delete all created RoleBindings)
kubectl delete foldertree my-organization

# Remove test namespaces
kubectl delete namespace prod-web prod-api staging prod-web-2

# Uninstall controller (if desired)
make undeploy  # For cluster deployment
# OR just stop the local process for development setup
```

---

**🎉 Congratulations!** You've successfully set up hierarchical RBAC with automatic inheritance. You just experienced the power of managing complex permissions through a single, declarative resource.

**Ready for more?** Check out the [Complete User Guide](GUIDE.md) for advanced features, security considerations, and production deployment patterns.
