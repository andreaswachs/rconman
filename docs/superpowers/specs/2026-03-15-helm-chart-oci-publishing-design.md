# Helm Chart OCI Publishing to GHCR - Design Specification

**Date:** 2026-03-15
**Status:** Draft
**Author:** Claude Code

## Overview

Enable automated publishing of the rconman Helm chart to GitHub Container Registry (GHCR) as an OCI artifact, with PR validation to ensure semantic versioning compliance when chart files are modified.

## Requirements

1. Publish Helm chart to GHCR as OCI artifact on every push to main and version tags
2. Use chart version (from Chart.yaml) as the OCI tag
3. Implement PR check that validates SEMVER bump when Helm chart files are modified
4. Prevent merging of PRs that modify chart without version bump
5. Keep chart version manually bumped (not auto-incremented)

## Architecture

### Workflow: PR Validation (`validate-helm-chart`)

**Trigger:** Pull requests to main branch

**Job Steps:**
1. Checkout code
2. Fetch main branch to compare against
3. Detect changes in `helm/rconman/` directory
4. If changes detected:
   - Extract old version from `main:helm/rconman/Chart.yaml` (using `yq` or similar)
   - Extract new version from PR branch `helm/rconman/Chart.yaml`
   - Validate version exists in PR (not empty/missing)
   - Parse both versions to ensure they match SEMVER format (major.minor.patch)
   - Compare versions to confirm bump occurred
   - Fail if: version unchanged, version deleted, or version is invalid format
5. If no changes to helm/ directory:
   - Pass silently (no validation needed)
6. If changes AND valid version bump:
   - Pass (allow PR to proceed)

**Status Check:** Adds "validate-helm-chart" as a required check before merge

### Workflow: Chart Publishing (`publish-helm-chart`)

**Trigger:** Push events to main branch and version tags (v*.*.*)

**Job Steps:**
1. Checkout code
2. Install Helm CLI (available in ubuntu-latest)
3. Package chart: `helm package helm/rconman/ -d /tmp/charts`
4. Authenticate to GHCR using GITHUB_TOKEN
5. Determine tag:
   - **On main branch push:** Use `latest-main`
   - **On version tag push:** Extract chart version from Chart.yaml and use as tag
6. Push to OCI registry: `helm push /tmp/charts/rconman-*.tgz oci://ghcr.io/${{ github.repository }}`
7. Create OCI artifact with appropriate tag

**Output:** Chart available at:
- `oci://ghcr.io/user/rconman:0.1.0` (semantic version)
- `oci://ghcr.io/user/rconman:latest-main` (latest from main)

**Usage:** Users can install with:
```bash
helm pull oci://ghcr.io/user/rconman --version 0.1.0
helm install myrelease oci://ghcr.io/user/rconman --version 0.1.0
```

## Data Flow

```
Developer creates PR modifying helm/rconman/values.yaml
    ↓
GitHub Actions: validate-helm-chart job triggered
    ↓
Compare Chart.yaml in main vs PR branch
    ↓
Check if version was bumped
    ↓
Validate SEMVER format (X.Y.Z)
    ↓
✅ PASS (version bumped) OR ❌ FAIL (no bump/invalid format)
    ↓
If FAIL: PR blocked, cannot merge
If PASS: PR allowed to proceed
    ↓
When merged to main: publish-helm-chart job triggers
    ↓
Package chart with new version
    ↓
Push to GHCR with version tag and latest-main tag
    ↓
Chart available for deployment
```

## Implementation Details

### Version Extraction & Validation

- Use `yq` (YAML parser) to extract `version:` field from Chart.yaml
- Regex validation: `^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$` (supports semantic versions like 1.2.3 and pre-release like 1.2.3-rc.1)
- Comparison: parse versions as integers (major*10000 + minor*100 + patch) for simple comparison

### GHCR Authentication

- Uses existing `GITHUB_TOKEN` secret (automatically provided by GitHub Actions)
- `helm registry login ghcr.io -u ${{ github.actor }} -p ${{ secrets.GITHUB_TOKEN }}`
- Same auth as existing Docker image publishing in build.yml

### Tag Strategy

- **Latest main:** `latest-main` tag ensures users can pull the bleeding edge
- **Version tag:** Semantic version (0.1.0) allows reproducible deployments
- Both tags point to same OCI artifact for a given push

## Error Handling

### Validation Job Failures

| Scenario | Action |
|----------|--------|
| Helm files changed, version unchanged | ❌ Fail: "Chart.yaml version must be bumped" |
| Helm files changed, version invalid format | ❌ Fail: "Chart.yaml version is invalid SEMVER" |
| Helm files changed, version decreased | ❌ Fail: "Version downgrade not allowed" |
| Helm files changed, version bumped correctly | ✅ Pass |
| No helm files changed | ✅ Pass (no validation needed) |

### Publishing Job Failures

| Scenario | Action |
|----------|--------|
| Helm push fails (auth issue) | ❌ Fail: "Failed to push chart to GHCR" |
| Chart packaging fails | ❌ Fail: "helm package failed" |
| GHCR registry unavailable | ❌ Fail: "GHCR unreachable" (auto-retry by GitHub Actions) |
| Successful publish | ✅ Pass: Chart available for pull |

## Testing & Validation

1. **Manual test of validation job:**
   - Create feature branch, modify helm/rconman/values.yaml
   - Push PR without version bump → validation should fail
   - Update Chart.yaml version from 0.1.0 to 0.2.0
   - Push again → validation should pass

2. **Manual test of publishing job:**
   - Merge PR to main
   - Verify GitHub Actions publishes chart
   - Test: `helm pull oci://ghcr.io/your-repo/rconman:0.2.0`
   - Verify chart can be installed in a test cluster

3. **Tag-based publishing:**
   - Create git tag `v0.2.0`
   - Verify publish-helm-chart runs
   - Verify chart published with version from Chart.yaml

## Dependencies

- `helm` CLI (included in ubuntu-latest)
- `yq` for YAML parsing (available in ubuntu-latest or installable via apt)
- GitHub Actions setup-buildx already in place for auth

## Files Modified

- `.github/workflows/build.yml` - add validate-helm-chart and extend publish-helm-chart jobs

## Rollout Plan

1. Add validate-helm-chart job to build.yml
2. Test in feature branch (validation should pass on PR with version bump)
3. Merge to main
4. Add publish-helm-chart job
5. Monitor first chart publish to GHCR
6. Document in README or deployment guide

## Success Criteria

✅ PRs modifying helm/ files without version bumps are blocked
✅ PRs modifying helm/ files with valid version bumps pass validation
✅ Chart publishes to GHCR on main branch push
✅ Chart publishes with semantic version tag
✅ Users can pull and install chart: `helm pull oci://ghcr.io/.../rconman:0.1.0`
✅ Chart available with both version tag and latest-main tag
