# Release Guide

This guide explains how to create releases for the FolderTree Controller, including building multi-platform container images and generating installable manifests.

## Prerequisites

Before creating a release, ensure:

1. **GitHub Permissions**: Repository must have GitHub Packages enabled
2. **Local Tools** (for manual releases):
   - Docker with BuildKit support
   - Docker buildx plugin
   - Make
   - kubectl (for testing manifests)

## Automated Release Process (Recommended)

The automated release process is triggered by pushing a version tag to GitHub. It will:
- Build multi-platform container images (amd64, arm64, s390x, ppc64le)
- Push images to GitHub Container Registry (ghcr.io)
- Generate the installable manifest
- Create a GitHub Release with the manifest attached

### Steps to Create a Release

1. **Ensure all changes are committed and pushed to main**:
   ```bash
   git status
   git push origin main
   ```

2. **Create and push a version tag**:
   ```bash
   # Create a version tag (follow semantic versioning)
   git tag -a v0.1.0 -m "Release v0.1.0"

   # Push the tag to GitHub
   git push origin v0.1.0
   ```

3. **Monitor the release workflow**:
   - Go to: https://github.com/mhenriks/kubernetes-foldertree-controller/actions
   - Watch the "Release" workflow run
   - It typically takes 5-10 minutes to complete

4. **Verify the release**:
   - Check: https://github.com/mhenriks/kubernetes-foldertree-controller/releases
   - Verify the `install.yaml` file is attached
   - Verify container images at: https://github.com/mhenriks?tab=packages

5. **Test the installation**:
   ```bash
   # Test on a cluster
   kubectl apply -f https://github.com/mhenriks/kubernetes-foldertree-controller/releases/download/v0.1.0/install.yaml

   # Verify the controller is running
   kubectl get pods -n foldertree-system
   ```

### Release Workflow Details

The GitHub Actions workflow (`.github/workflows/release.yml`) performs:

1. **Checkout**: Fetches the repository code
2. **Setup**: Configures Go and Docker Buildx
3. **Login**: Authenticates to ghcr.io using GitHub token
4. **Build**: Creates multi-platform images using `make docker-buildx`
5. **Tag**: Tags images with both version tag and `:latest`
6. **Generate**: Creates installable manifest with `make build-installer`
7. **Release**: Creates GitHub release with manifest attached

## Manual Release Process

If you need to create a release manually:

### 1. Build and Push Container Images

```bash
# Set your version
export VERSION=v0.1.0
export IMG=ghcr.io/mhenriks/foldertree-controller:${VERSION}

# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u mhenriks --password-stdin

# Build and push multi-platform images
make docker-buildx IMG=${IMG}

# Also tag as latest (optional)
docker buildx imagetools create ${IMG} --tag ghcr.io/mhenriks/foldertree-controller:latest
```

### 2. Generate Installable Manifest

```bash
# Generate the manifest with the correct image tag
export IMG=ghcr.io/mhenriks/foldertree-controller:${VERSION}
make build-installer

# The manifest will be created at: dist/install.yaml
```

### 3. Create GitHub Release

```bash
# Create a git tag
git tag -a ${VERSION} -m "Release ${VERSION}"
git push origin ${VERSION}

# Create the release using GitHub CLI
gh release create ${VERSION} \
  --title "FolderTree Controller ${VERSION}" \
  --notes "Release notes here" \
  dist/install.yaml
```

Or create the release manually through the GitHub web interface:
1. Go to: https://github.com/mhenriks/kubernetes-foldertree-controller/releases/new
2. Select the tag you created
3. Add release notes
4. Upload `dist/install.yaml`
5. Publish the release

## Container Image Management

### Image Naming Convention

- **Versioned**: `ghcr.io/mhenriks/foldertree-controller:v0.1.0`
- **Latest**: `ghcr.io/mhenriks/foldertree-controller:latest`
- **SHA**: `ghcr.io/mhenriks/foldertree-controller:sha-abc123def`

### Supported Platforms

The release workflow builds images for:
- `linux/amd64` - Intel/AMD 64-bit
- `linux/arm64` - ARM 64-bit (Apple Silicon, ARM servers)
- `linux/s390x` - IBM System z
- `linux/ppc64le` - PowerPC 64-bit Little Endian

### Making Images Public

By default, GitHub Container Registry images are private. To make them public:

1. Go to: https://github.com/users/mhenriks/packages/container/foldertree-controller/settings
2. Scroll to "Danger Zone"
3. Click "Change visibility"
4. Select "Public"

## Versioning Guidelines

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** version (v1.0.0 → v2.0.0): Incompatible API changes
- **MINOR** version (v0.1.0 → v0.2.0): Add functionality in a backward compatible manner
- **PATCH** version (v0.1.0 → v0.1.1): Backward compatible bug fixes

### Pre-release Versions

For pre-releases, use suffixes:
- Alpha: `v0.1.0-alpha.1`
- Beta: `v0.1.0-beta.1`
- RC: `v0.1.0-rc.1`

Mark these as "pre-release" in GitHub when creating the release.

## Troubleshooting

### Docker Buildx Issues

If `make docker-buildx` fails:

```bash
# Check buildx is installed
docker buildx version

# Create a new builder if needed
docker buildx create --name folders-builder --use
docker buildx inspect --bootstrap
```

### Authentication Issues

If image push fails:

```bash
# Verify you're logged in
docker login ghcr.io

# Check token permissions (needs: write:packages, read:packages)
# Create a new token at: https://github.com/settings/tokens
```

### Manifest Generation Issues

If `make build-installer` fails:

```bash
# Ensure all prerequisites are met
make manifests generate

# Check kustomize is available
make kustomize
./bin/kustomize version
```

### Permission Issues on GitHub

If the workflow fails with permission errors:

1. Go to: Settings → Actions → General
2. Under "Workflow permissions", select "Read and write permissions"
3. Ensure "Allow GitHub Actions to create and approve pull requests" is checked

## Testing a Release

Before announcing a release, test it:

```bash
# Create a test cluster
kind create cluster --name release-test

# Install the release
kubectl apply -f https://github.com/mhenriks/kubernetes-foldertree-controller/releases/download/v0.1.0/install.yaml

# Wait for controller to be ready
kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n foldertree-system --timeout=120s

# Test with a sample FolderTree
kubectl apply -f config/samples/rbac_v1alpha1_foldertree.yaml

# Verify it works
kubectl get foldertrees

# Cleanup
kind delete cluster --name release-test
```

## Rollback

If you need to rollback a release:

```bash
# Delete the bad release and tag
gh release delete v0.1.0 --yes
git tag -d v0.1.0
git push origin :refs/tags/v0.1.0

# Delete the container images
# Go to: https://github.com/users/mhenriks/packages/container/foldertree-controller
# Delete the specific version
```

## Release Checklist

Before creating a release:

- [ ] All tests pass (`make test`)
- [ ] E2E tests pass (`make test-e2e`)
- [ ] Linting passes (`make lint`)
- [ ] Documentation is updated
- [ ] CHANGELOG or release notes prepared
- [ ] Version number follows semantic versioning
- [ ] All changes are merged to main branch

After creating a release:

- [ ] Verify the GitHub release was created
- [ ] Verify container images are published to ghcr.io
- [ ] Verify install.yaml is attached to the release
- [ ] Test installation on a fresh cluster
- [ ] Update documentation with new version
- [ ] Announce the release (if applicable)

## Continuous Delivery (Future)

For automatic releases on every merge to main, you could:

1. Use a tool like [semantic-release](https://github.com/semantic-release/semantic-release)
2. Configure automatic version bumping based on commit messages
3. Auto-generate changelogs from commit history

This would require modifying the release workflow to run on pushes to main instead of tags.

## Resources

- [GitHub Container Registry Documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Docker Buildx Documentation](https://docs.docker.com/buildx/working-with-buildx/)
- [Semantic Versioning](https://semver.org/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github)
