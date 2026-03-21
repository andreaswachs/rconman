# rconman — Contributor Guidelines

## Version Bumping Rules

### Application code changes (`image.yaml`)
Any change to Go source code, views, or other files that affect the built binary **must** include a patch version bump in `image.yaml`.

### Helm chart changes (`helm/rconman/Chart.yaml`)
Any change to files under `helm/rconman/` **must** include a patch version bump to `version` in `helm/rconman/Chart.yaml`.
