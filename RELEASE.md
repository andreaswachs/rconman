# Release Workflow

This document explains how to build, push images to GitHub Container Registry (GHCR), and manage releases for rconman.

## Automated Releases (GitHub Actions)

The CI/CD pipeline automatically handles releases:

### Build on Every Push to main
When you push to the `main` branch, GitHub Actions:
1. Runs tests
2. Builds the Docker image
3. Pushes to `ghcr.io/<owner>/rconman:main`
4. Pushes to `ghcr.io/<owner>/rconman:sha-<commit-hash>`

### Semantic Versioning with Tags
When you create a git tag like `v1.2.3`:
```bash
git tag v1.2.3
git push origin v1.2.3
```

GitHub Actions automatically:
1. Builds multiarch image (linux/amd64, linux/arm64)
2. Pushes to `ghcr.io/<owner>/rconman:v1.2.3`
3. Pushes to `ghcr.io/<owner>/rconman:1.2` (major.minor)
4. Pushes to `ghcr.io/<owner>/rconman:latest`
5. Builds a Helm chart and pushes to GHCR OCI registry

## Local Release Workflow

### Prerequisites

Ensure you're logged into Docker with GHCR access:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u <username> --password-stdin
```

Or use `gh`:
```bash
gh auth login
gh auth token | docker login ghcr.io -u <username> --password-stdin
```

### Build and Push Docker Image

**Option 1: Push with custom tag**
```bash
# Default tag is 'latest'
make docker-push

# Or specify a custom tag (e.g., version)
make docker-push IMAGE_TAG=v1.2.3
```

**Option 2: Push both latest and version tags**
```bash
make docker-push-latest IMAGE_TAG=v1.2.3
```

This pushes:
- `ghcr.io/your-org/rconman:latest`
- `ghcr.io/your-org/rconman:v1.2.3`

**Option 3: Multiarch build and push (amd64 + arm64)**
```bash
make docker-buildx-push IMAGE_TAG=v1.2.3
```

Requires Docker Buildx. Build locally:
```bash
# First, create a builder if you don't have one
docker buildx create --name mybuilder --use

# Then push
make docker-buildx-push IMAGE_TAG=v1.2.3
```

### Helm Chart Release

Helm charts are released via GitHub Actions when you tag a release. To manually package:

```bash
helm package helm/rconman -d helm/releases
```

## Release Checklist

Before releasing a new version:

### 1. Prepare the Release Branch
```bash
# Create release branch
git checkout -b release/v1.2.3
```

### 2. Update Version References
- Update `helm/rconman/Chart.yaml`:
  ```yaml
  version: 1.2.3
  appVersion: "1.2.3"
  ```
- Update any documentation referencing the version

### 3. Test Everything
```bash
make test          # Run tests
make lint          # Check code quality
make docker-build  # Verify Docker build
make helm-lint     # Verify Helm chart
```

### 4. Create Release Commit
```bash
git add helm/rconman/Chart.yaml # and any other version files
git commit -m "release: v1.2.3"
```

### 5. Create Tag
```bash
git tag -a v1.2.3 -m "Release version 1.2.3"
```

### 6. Push to main and Tag
```bash
git push origin release/v1.2.3  # or main if on main
git push origin v1.2.3          # Push the tag
```

### 7. Create GitHub Release (Optional)

Go to https://github.com/your-org/rconman/releases and create a release from the tag with release notes.

## Image Tagging Strategy

The GitHub Actions workflow uses this tagging strategy:

| Trigger | Tags |
|---------|------|
| Push to main | `main`, `sha-<commit>` |
| Tag v1.2.3 | `v1.2.3`, `1.2`, `latest`, `sha-<commit>` |

## Using Released Images

### From main Branch
```bash
docker pull ghcr.io/your-org/rconman:main
```

### From Specific Version
```bash
docker pull ghcr.io/your-org/rconman:v1.2.3
```

### With Kubernetes
Update your Helm values:
```yaml
image:
  repository: ghcr.io/your-org/rconman
  tag: v1.2.3
```

Then install:
```bash
helm install rconman helm/rconman \
  --set image.tag=v1.2.3
```

## Troubleshooting

### Docker Login Failed
```bash
# Verify token is valid
gh auth token

# Re-login
gh auth login
```

### GHCR Push Denied
Ensure:
1. Your GitHub token has `write:packages` permission
2. You're authenticated: `docker login ghcr.io`
3. The repository is public (or your token has private repo access)

### Multiarch Push Not Working
```bash
# Check if buildx is set up
docker buildx ls

# If not, create a builder
docker buildx create --name multiarch --use
docker buildx ls
```

### Image Tag Calculations Wrong
The REPO variable is auto-calculated from git remote:
```bash
# To override, explicitly specify:
make docker-push REPO=ghcr.io/my-org/rconman IMAGE_TAG=v1.2.3
```

## GitHub Actions Secrets

The build workflow uses `secrets.GITHUB_TOKEN` (automatically available in Actions) to:
- Authenticate to GHCR
- Build cache

No additional secrets are needed.

## Advanced: Manual Helm Chart Release

If GitHub Actions doesn't release the Helm chart, you can do it manually:

```bash
# Package the chart
helm package helm/rconman -d /tmp/charts

# Push using OCI registry (requires helm >= 3.7)
helm push /tmp/charts/rconman-1.2.3.tgz oci://ghcr.io/your-org/helm-charts
```

## References

- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Docker Buildx](https://docs.docker.com/buildx/working-with-buildx/)
- [Helm OCI Support](https://helm.sh/docs/topics/registries/)
