# Helm Chart Publishing

The rconman Helm chart is automatically published to GHCR as an OCI artifact.

## Installing

```bash
helm pull oci://ghcr.io/<OWNER>/charts/rconman --version 0.1.0
helm install rconman oci://ghcr.io/<OWNER>/charts/rconman --version 0.1.0
```

Replace `<OWNER>` with the GitHub organization or user that owns the repository.

## Publishing

The chart is published automatically by the `publish-helm-chart` GitHub Actions job when a push to `main` includes changes to `helm/rconman/`.

## Version Management

Chart versions are managed manually in `helm/rconman/Chart.yaml`.

### PR Requirement

Any PR that modifies files in `helm/rconman/` **must** bump the `version` field in `Chart.yaml` following [semantic versioning](https://semver.org/) (e.g. `0.1.0` → `0.1.1`). The `validate-helm-chart` CI job will block the PR otherwise.

To bump the version:

```yaml
# helm/rconman/Chart.yaml
version: 0.2.0       # was 0.1.0
appVersion: "0.2.0"  # update to match
```

## Troubleshooting

**Validation fails on PR:** Ensure `version:` in `Chart.yaml` is higher than on `main` and is valid SEMVER (`X.Y.Z`).

**Publish job skipped:** The job only runs when `helm/rconman/` files change. If only app code changed, the publish job exits early.

**`helm pull` fails:** Ensure you are using the OCI prefix: `oci://ghcr.io/...`. For private repositories, authenticate first: `helm registry login ghcr.io`.
