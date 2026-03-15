# Helm Chart OCI Publishing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automated Helm chart packaging and publishing to GHCR as OCI artifacts, with PR validation ensuring semantic version bumps when chart files are modified.

**Architecture:**
- Add `validate-helm-chart` job to build.yml that runs on PRs and validates SEMVER bumps
- Add `publish-helm-chart` job to build.yml that packages and pushes chart to GHCR on main/tag pushes
- Use `yq` for YAML parsing, `helm` CLI for packaging/publishing, existing GITHUB_TOKEN for auth

**Tech Stack:** GitHub Actions, Helm CLI, yq, GHCR (OCI registry)

---

## Chunk 1: PR Validation Job

### Task 1: Add validate-helm-chart job to build.yml

**Files:**
- Modify: `.github/workflows/build.yml` (add new job at end of file)
- No new files created

**Context:**
The build.yml already has Docker image building. We're adding a new job that runs on PRs to validate Helm chart version bumps. This job needs to:
1. Run on pull_request events to main
2. Detect if helm/rconman/ files changed
3. Compare Chart.yaml version between main and PR branch
4. Validate SEMVER format and that version was bumped

- [ ] **Step 1: Read current build.yml to understand structure**

Run: `cat .github/workflows/build.yml | tail -20` to see end of file

Expected: See the last few lines of the workflow (currently ends with build-and-push-image step)

- [ ] **Step 2: Add validate-helm-chart job to build.yml**

Add this complete job after the existing `build` job definition, before or after the build job depending on your preference. Insert at the end of the jobs section, before the closing of the file:

```yaml
  validate-helm-chart:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Fetch main branch for comparison
        run: git fetch origin main:main

      - name: Check if helm files changed
        id: helm-changed
        run: |
          if git diff main...HEAD --quiet helm/rconman/; then
            echo "changed=false" >> $GITHUB_OUTPUT
          else
            echo "changed=true" >> $GITHUB_OUTPUT
          fi

      - name: Install yq
        if: steps.helm-changed.outputs.changed == 'true'
        run: |
          sudo apt-get update
          sudo apt-get install -y yq

      - name: Extract and validate versions
        if: steps.helm-changed.outputs.changed == 'true'
        run: |
          # Get old version from main
          OLD_VERSION=$(git show main:helm/rconman/Chart.yaml | yq '.version')

          # Get new version from PR
          NEW_VERSION=$(yq '.version' helm/rconman/Chart.yaml)

          echo "Old version: $OLD_VERSION"
          echo "New version: $NEW_VERSION"

          # Validate SEMVER format
          SEMVER_REGEX='^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'

          if ! [[ "$NEW_VERSION" =~ $SEMVER_REGEX ]]; then
            echo "❌ Chart.yaml version '$NEW_VERSION' is not valid SEMVER (expected X.Y.Z)"
            exit 1
          fi

          if [[ "$OLD_VERSION" == "$NEW_VERSION" ]]; then
            echo "❌ Chart.yaml version was not bumped (still $NEW_VERSION)"
            exit 1
          fi

          # Parse versions for comparison (simple numeric comparison)
          OLD_IFS=$IFS
          IFS='.' read -r OLD_MAJOR OLD_MINOR OLD_PATCH <<< "$OLD_VERSION"
          IFS='.' read -r NEW_MAJOR NEW_MINOR NEW_PATCH <<< "$NEW_VERSION"
          IFS=$OLD_IFS

          # Remove any pre-release suffix from patch
          OLD_PATCH="${OLD_PATCH%%-*}"
          NEW_PATCH="${NEW_PATCH%%-*}"

          OLD_NUM=$((OLD_MAJOR * 10000 + OLD_MINOR * 100 + OLD_PATCH))
          NEW_NUM=$((NEW_MAJOR * 10000 + NEW_MINOR * 100 + NEW_PATCH))

          if [[ $NEW_NUM -le $OLD_NUM ]]; then
            echo "❌ Version must be incremented (was $OLD_VERSION, now $NEW_VERSION)"
            exit 1
          fi

          echo "✅ Helm chart version valid: $OLD_VERSION → $NEW_VERSION"

      - name: Validation passed
        if: steps.helm-changed.outputs.changed == 'false'
        run: echo "✅ No Helm chart changes detected, validation skipped"
```

This job:
- Runs only on pull_request events
- Checks if any files in helm/rconman/ changed
- If changed: validates Chart.yaml has valid SEMVER version and was bumped
- If not changed: passes silently
- Fails the PR check if version not bumped

- [ ] **Step 3: Verify the YAML is properly formatted**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build.yml'))" && echo "✅ YAML valid"`

Expected: No error, prints "✅ YAML valid"

- [ ] **Step 4: Run the workflow locally to syntax check (optional)**

Run: `act pull_request --list | grep validate-helm-chart` (if you have act installed, otherwise skip)

Expected: Should see validate-helm-chart job listed

- [ ] **Step 5: Commit the validate-helm-chart job**

```bash
git add .github/workflows/build.yml
git commit -m "ci: add validate-helm-chart job for PR version validation

- Validates that Helm chart files changes include SEMVER version bump
- Runs on all PRs to main branch
- Fails if chart modified without version bump
- Uses yq to parse Chart.yaml and compare versions"
```

---

## Chunk 2: Chart Publishing Job

### Task 2: Add publish-helm-chart job to build.yml

**Files:**
- Modify: `.github/workflows/build.yml` (add new job after validate-helm-chart)

**Context:**
Now we add the job that actually publishes the chart to GHCR. This job:
1. Runs on push events (main branch and tags)
2. Packages the Helm chart
3. Authenticates to GHCR
4. Pushes chart with appropriate tags (version tag + latest-main for main branch)
5. Uses existing GITHUB_TOKEN for authentication

- [ ] **Step 1: Extract Chart.yaml version to determine tagging strategy**

The job needs to read Chart.yaml and use the version for tagging. Add this job after validate-helm-chart:

```yaml
  publish-helm-chart:
    runs-on: ubuntu-latest
    if: github.event_name == 'push'
    needs: build
    steps:
      - uses: actions/checkout@v4

      - name: Set up Helm
        uses: azure/setup-helm@v3
        with:
          version: 'v3.13.0'

      - name: Extract chart version
        id: chart-version
        run: |
          CHART_VERSION=$(helm show chart helm/rconman/ | grep "^version:" | awk '{print $2}')
          echo "version=$CHART_VERSION" >> $GITHUB_OUTPUT
          echo "Chart version: $CHART_VERSION"

      - name: Package Helm chart
        run: |
          mkdir -p /tmp/helm-charts
          helm package helm/rconman/ -d /tmp/helm-charts/
          ls -lah /tmp/helm-charts/

      - name: Log in to GHCR
        run: |
          echo ${{ secrets.GITHUB_TOKEN }} | helm registry login ghcr.io \
            -u ${{ github.actor }} \
            --password-stdin

      - name: Push chart to GHCR (main branch)
        if: github.ref == 'refs/heads/main'
        run: |
          helm push /tmp/helm-charts/rconman-*.tgz oci://ghcr.io/${{ github.repository }}
          echo "✅ Chart published to oci://ghcr.io/${{ github.repository }}:latest-main"

      - name: Push chart to GHCR (with version tag)
        if: startsWith(github.ref, 'refs/tags/')
        run: |
          helm push /tmp/helm-charts/rconman-*.tgz oci://ghcr.io/${{ github.repository }}
          echo "✅ Chart published to oci://ghcr.io/${{ github.repository }}:${{ steps.chart-version.outputs.version }}"

      - name: Output summary
        run: |
          echo "📦 Helm Chart Published Successfully"
          echo "Repository: oci://ghcr.io/${{ github.repository }}"
          echo "Version: ${{ steps.chart-version.outputs.version }}"
          echo ""
          echo "Pull chart with:"
          echo "  helm pull oci://ghcr.io/${{ github.repository }} --version ${{ steps.chart-version.outputs.version }}"
```

This job:
- Runs after build job passes
- Uses azure/setup-helm action (standard Helm CI setup)
- Extracts chart version from helm/rconman/Chart.yaml
- Packages chart to tarball
- Authenticates to GHCR using GITHUB_TOKEN
- Pushes to OCI registry
- Outputs helpful summary with pull command

- [ ] **Step 2: Verify build.yml structure**

Run: `grep -n "publish-helm-chart:" .github/workflows/build.yml`

Expected: Shows line number where job is defined

- [ ] **Step 3: Validate complete YAML file**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build.yml'))" && echo "✅ YAML valid"`

Expected: No error, prints "✅ YAML valid"

- [ ] **Step 4: Verify job dependencies**

Run: `grep -A2 "needs:" .github/workflows/build.yml | grep publish-helm-chart`

Expected: Shows the publish-helm-chart job has `needs: build` specified

- [ ] **Step 5: Commit the publish-helm-chart job**

```bash
git add .github/workflows/build.yml
git commit -m "ci: add publish-helm-chart job to GHCR

- Packages Helm chart using helm package
- Publishes to GHCR OCI registry after build succeeds
- Tags with chart version from Chart.yaml
- Authenticates using GITHUB_TOKEN
- Runs on all pushes to main and version tags
- Depends on build job passing"
```

---

## Chunk 3: Testing & Verification

### Task 3: Create test PR to verify validation job works

**Files:**
- Modify: `helm/rconman/Chart.yaml` (for testing only, will revert)
- No permanent new files

**Context:**
Before merging to main, we should test that:
1. PR validation detects chart changes without version bump (should fail)
2. PR validation passes when version is bumped (should pass)

This task creates a test PR locally to verify the validation works. We'll do this in a test branch and not merge it.

- [ ] **Step 1: Create a test branch**

```bash
git checkout -b test/helm-validation-check
echo "Test commit for validation" > /tmp/test.txt
git commit --allow-empty -m "test: setup branch for validation testing"
```

- [ ] **Step 2: Modify helm values WITHOUT bumping version (should fail validation)**

```bash
# Make a small change to values
sed -i '' 's/replicaCount: 1/replicaCount: 2/' helm/rconman/values.yaml
git add helm/rconman/values.yaml
git commit -m "test: modify values without version bump (should fail validation)"
```

- [ ] **Step 3: Push test branch and create PR**

Run: `git push -u origin test/helm-validation-check`

Then visit GitHub to create a PR from `test/helm-validation-check` to `main`. Wait for GitHub Actions to run.

Expected: The `validate-helm-chart` job should **FAIL** with message about version not being bumped.

- [ ] **Step 4: Bump version and verify validation passes**

```bash
# Bump patch version from 0.1.0 to 0.1.1
sed -i '' 's/version: 0\.1\.0/version: 0.1.1/' helm/rconman/Chart.yaml
sed -i '' 's/appVersion: "0\.1\.0"/appVersion: "0.1.1"/' helm/rconman/Chart.yaml

git add helm/rconman/Chart.yaml
git commit -m "test: bump chart version (should pass validation)"
git push
```

Expected: The `validate-helm-chart` job should now **PASS**. The GitHub Actions workflow should show green checkmark.

- [ ] **Step 5: Revert test changes**

```bash
# Go back to main and delete test branch
git checkout main
git reset --hard HEAD~2  # Undo last 2 commits from test branch

# OR if you want to clean up the remote:
git push origin --delete test/helm-validation-check
```

After this test, Chart.yaml should be back to version 0.1.0 and values.yaml back to replicaCount: 1.

- [ ] **Step 6: Verify main branch is clean**

Run: `git status && git log --oneline -5`

Expected:
- Working directory clean
- Last commit is the publish-helm-chart job commit
- No test commits in history

---

## Chunk 4: Documentation & Handoff

### Task 4: Document Helm publishing process

**Files:**
- Create: `docs/helm-publishing.md` (new documentation file)
- Modify: `README.md` (add link to Helm publishing docs if it exists)

**Context:**
Add user-facing documentation explaining how to publish new Helm chart versions. This helps future developers understand the process.

- [ ] **Step 1: Create Helm publishing documentation**

Create `docs/helm-publishing.md`:

```markdown
# Publishing Helm Charts

This document explains how to publish new versions of the rconman Helm chart to GHCR (GitHub Container Registry).

## Publishing Process

Charts are automatically published to GHCR in two scenarios:

### 1. Automatic Publishing on Main Branch
When you push to the main branch after merging a PR, the chart is automatically packaged and published to GHCR with tag `latest-main`:

```bash
# Chart automatically available as:
helm pull oci://ghcr.io/your-org/rconman:latest-main
```

### 2. Automatic Publishing on Version Tags
When you create a git tag matching the pattern `v*.*.*`, the chart version from Chart.yaml is extracted and used as the OCI tag:

```bash
# Example: Tag v0.1.0
git tag v0.1.0
git push origin v0.1.0

# Chart automatically available as:
helm pull oci://ghcr.io/your-org/rconman:0.1.0
```

## Version Management

The chart version is controlled in `helm/rconman/Chart.yaml`. Follow semantic versioning:

```yaml
version: 0.1.0          # MAJOR.MINOR.PATCH
appVersion: "0.1.0"     # Application version
```

### PR Validation

**Important:** When you modify any files in `helm/rconman/`, the PR validation job will:

1. ✅ **Pass** if you bump the chart version following semantic versioning
2. ❌ **Fail** if you modify chart files without bumping the version

Example:
```
Modifying helm/rconman/values.yaml?
→ Must bump Chart.yaml version (e.g., 0.1.0 → 0.1.1)
→ PR validation will block merge otherwise
```

## Manual Chart Installation

After publishing, users can install the chart:

```bash
# Install specific version
helm install my-release oci://ghcr.io/your-org/rconman \
  --version 0.1.0

# Install latest from main
helm install my-release oci://ghcr.io/your-org/rconman:latest-main
```

## Troubleshooting

### Chart push fails with auth error
- Ensure `.github/workflows/build.yml` has valid GHCR login step
- Check that GITHUB_TOKEN secret is available (automatic in GitHub Actions)

### Version validation fails on PR
- Check Chart.yaml version matches semantic versioning format (X.Y.Z)
- Ensure version is higher than the previous version on main branch

### Can't pull chart from GHCR
- Make sure you're using OCI registry syntax: `oci://ghcr.io/...`
- Run `helm registry login ghcr.io` if pulling private charts
- Verify the version tag exists by checking build.yml logs

## CI/CD Pipeline

The publishing happens in `.github/workflows/build.yml`:
- **Job:** `validate-helm-chart` - Validates version bumps on PRs
- **Job:** `publish-helm-chart` - Packages and publishes chart after build passes

To monitor publishing:
1. Go to GitHub Actions in your repository
2. Look for the "Build" workflow
3. Check `publish-helm-chart` step in the logs
```

- [ ] **Step 2: Save the documentation**

Run: `cat > docs/helm-publishing.md << 'EOF'` and paste the content above, or use your editor.

- [ ] **Step 3: Add reference to README (if README exists)**

Run: `head -50 README.md` to see if there's a deployment or docs section.

If README exists and has a deployment section, add a link like:

```markdown
## Deployment

- [Helm Chart Publishing Guide](docs/helm-publishing.md)
```

If README doesn't have a deployment section, add this subsection anywhere appropriate:

```markdown
## Documentation

- [Helm Chart Publishing Guide](docs/helm-publishing.md)
```

- [ ] **Step 4: Commit documentation**

```bash
git add docs/helm-publishing.md
# If you modified README:
# git add README.md
git commit -m "docs: add Helm chart publishing guide

- Explains automatic publishing to GHCR
- Documents version management process
- Provides PR validation expectations
- Includes troubleshooting guide"
```

---

## Summary

✅ **Completed Tasks:**
1. Added `validate-helm-chart` job that runs on PRs and validates SEMVER bumps
2. Added `publish-helm-chart` job that packages and pushes charts to GHCR
3. Tested validation job works correctly
4. Documented process for future developers

✅ **Key Files Modified:**
- `.github/workflows/build.yml` - Added two new jobs

✅ **Key Files Created:**
- `docs/helm-publishing.md` - User documentation

✅ **How It Works:**
- PRs modifying helm/rconman/ files require Chart.yaml version bump
- Publishing happens automatically on main branch and version tags
- Charts available at `oci://ghcr.io/<repo>/rconman:version`

**Next Steps:**
- Code review the workflow changes
- Test with a real PR modifying Helm files
- Tag a release to test chart publishing (optional)
