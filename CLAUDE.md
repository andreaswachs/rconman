# rconman — Contributor Guidelines

## Version Bumping Rules

### Application code changes (`image.yaml`)
Any change to Go source code, views, or other files that affect the built binary **must** include a patch version bump in `image.yaml`.

### Helm chart changes (`helm/rconman/Chart.yaml`)
Any change to files under `helm/rconman/` **must** include a patch version bump to `version` in `helm/rconman/Chart.yaml`.

### App version and image tag alignment
The tag configured for the image in `image.yaml` needs to be aligned at all times with the `appVersion` field in the Chrat configuration.
