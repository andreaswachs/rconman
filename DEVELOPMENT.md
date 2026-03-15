# rconman Development Guide

## Quick Start

### 1. Install Development Tools (One-Time Setup)

```bash
make install-tools
```

This installs:
- **air** — Live reload for Go development
- **golangci-lint** — Code linting
- **goimports** — Import formatting

### 2. Configure Local Credentials (One-Time Setup)

For testing with real OAuth2 credentials:

```bash
cp config.dev.yaml config.dev.yaml
# Edit config.dev.yaml and add your real credentials
```

**What to add:**
- Google OAuth2 `client_id` and `client_secret` (from [Google Cloud Console](https://console.cloud.google.com))
- Your email in `email_allowlist`
- Minecraft server connection details

⚠️ **Security:** `config.dev.yaml` is git-ignored. It's safe to commit sensitive credentials here — they'll stay local.

### 3. Start Developing

```bash
make dev
```

This will:
- Auto-detect `config.dev.yaml` (if it exists) or use `config.example.yaml`
- Generate code from templ templates
- Build Tailwind CSS
- Start the rconman server with live reloading
- Watch for changes in `.go`, `.templ`, `.html`, `.css` files
- Rebuild and restart the server automatically when files change

The server will be available at `http://localhost:8080`.

**Config priority:**
1. `config.dev.yaml` (your local dev config with real credentials)
2. `config.example.yaml` (fallback with placeholder values)

### 4. Stop Development Server

Press `Ctrl+C` to gracefully shutdown the server.

## Setting Up OAuth2 Credentials

To test the full authentication flow with real OAuth2:

### Google OAuth2 (Recommended)

1. **Create OAuth2 credentials** in [Google Cloud Console](https://console.cloud.google.com):
   - Go to **APIs & Services** → **Credentials**
   - Create a new **OAuth 2.0 Client ID** (Web application)
   - Authorized redirect URIs: `http://localhost:8080/auth/callback`

2. **Copy credentials to config.dev.yaml:**
   ```yaml
   auth:
     oidc:
       client_id:
         value: "YOUR_CLIENT_ID.apps.googleusercontent.com"
       client_secret:
         value: "YOUR_CLIENT_SECRET"
   ```

3. **Add your email to the allowlist:**
   ```yaml
   admin:
     email_allowlist:
       - "your-email@gmail.com"
   ```

4. **Start dev server:**
   ```bash
   make dev
   ```

5. **Visit http://localhost:8080** → Click login → Authenticate with Google

### Other OIDC Providers

rconman supports any OIDC-compliant provider. Update `issuer_url` in `config.dev.yaml`:

```yaml
auth:
  oidc:
    issuer_url: "https://your-oidc-provider.com"
    client_id:
      value: "your-client-id"
    client_secret:
      value: "your-client-secret"
```

### Testing Without OAuth2

If you don't want to set up OAuth2, use the Docker development config:

```bash
make docker-run
```

This uses dev credentials without requiring OAuth2 setup.

## Development Tasks

### Running Tests

```bash
# Run full test suite once
make test

# Run tests in watch mode (re-run on file changes)
make test-watch
```

### Building for Production

```bash
# Build optimized binary with all assets
make build

# Output: ./rconman (single executable, no external dependencies)
```

### Code Quality

```bash
# Auto-format code
make format

# Run linters
make lint

# Both format and lint
make format && make lint
```

### CSS Development

If you're only working on CSS/UI:

```bash
# Watch and rebuild Tailwind CSS only
make dev-css
```

This rebuilds `web/static/app.css` whenever you modify files in `internal/views/` or `web/`.

### Docker Development

```bash
# Build Docker image
make docker-build

# Build and run in Docker
make docker-run
```

The Docker container will be available at `http://localhost:8080` with the example configuration.

### Kubernetes/Helm

```bash
# Validate Helm chart
make helm-lint

# Preview rendered Helm templates
make helm-template
```

### E2E Testing

```bash
# Run full e2e test suite (requires Docker and Kind)
make e2e
```

This:
1. Creates a Kind cluster
2. Builds Docker images
3. Installs rconman via Helm
4. Runs the e2e test suite
5. Cleans up the cluster

## Project Structure

```
rconman/
├── cmd/rconman/           # Application entry point
├── internal/
│   ├── config/            # Configuration loading and validation
│   ├── auth/              # OIDC/OAuth2 authentication
│   ├── rcon/              # RCON protocol implementation
│   ├── server/            # HTTP server setup (chi)
│   ├── handlers/          # HTTP request handlers
│   ├── views/             # Templ templates
│   ├── store/             # SQLite database
│   └── model/             # Shared types
├── web/                   # Frontend assets
│   ├── static/            # Compiled Tailwind CSS (generated)
│   ├── input.css          # Tailwind directives
│   ├── tailwind.config.js # Tailwind configuration
│   └── package.json       # Node.js dependencies
├── helm/rconman/          # Kubernetes Helm chart
├── test/
│   ├── e2e/               # E2E test suite
│   ├── mock-rcon/         # Mock RCON server for testing
│   └── kind/              # Kind cluster setup/teardown
├── .air.toml              # Air live-reload configuration
├── Makefile               # Development tasks
└── config.example.yaml    # Example configuration

```

## Configuration for Development

The example configuration (`config.example.yaml`) uses:
- **OIDC Issuer:** `https://accounts.google.com` (requires setup)
- **Secrets:** Loaded from environment variables or inline placeholders

For local development, you can modify `config.example.yaml` to use inline secret values:

```yaml
server:
  session_secret:
    value: "32-byte-minimum-secret-key-for-dev"

auth:
  oidc:
    client_id:
      value: "your-client-id"
    client_secret:
      value: "your-client-secret"

minecraft:
  servers:
    - name: "Local Server"
      id: "local"
      rcon:
        host: "localhost"
        port: 25575
        password:
          value: "rcon-password"
```

## Troubleshooting

### "command not found: air"

Install air:
```bash
go install github.com/cosmtrek/air@latest
```

Or run:
```bash
make install-tools
```

### "npm: command not found"

Install Node.js 22+ from https://nodejs.org/

### Templ files not updating

The `air` tool watches for `.go` files by default. Templ changes trigger Go file recompilation via `go generate`:

```bash
go generate ./...
```

This happens automatically in `make dev`. If changes don't apply:
1. Save your `.templ` file
2. Air detects the generated `.go` file change
3. Server rebuilds within 1 second

### Port 8080 already in use

Change the config port or kill the process:
```bash
lsof -ti:8080 | xargs kill -9
```

### Database lock errors

Remove old database files:
```bash
make clean
```

This removes `rconman.db` and WAL files.

## Performance Tips

### During Development

- Use `make dev` for live reloading
- Use `make test-watch` for TDD workflow
- Separate CSS work with `make dev-css` if needed

### Before Committing

```bash
make format       # Auto-format code
make lint         # Check for issues
make test         # Ensure tests pass
```

### Building for Production

```bash
make build        # Optimized binary
make docker-build # Container image
```

## Useful Commands

```bash
# View all available targets
make help

# Check what would be built
helm template rconman helm/rconman

# Validate configuration file
./rconman -config config.example.yaml  # Press Ctrl+C after startup

# Run specific test
go test -v -run TestName ./internal/package

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Next Steps

1. **Set up OIDC:** Configure OAuth2 credentials in `config.example.yaml`
2. **Connect to RCON:** Point to your Minecraft server in the config
3. **Create templates:** Add command templates for your use case
4. **Deploy:** Use `make docker-build` and push to registry, then deploy with Helm

## Resources

- [Go](https://golang.org/doc/)
- [Templ](https://templ.guide/)
- [Tailwind CSS](https://tailwindcss.com/)
- [Chi Router](https://github.com/go-chi/chi)
- [Helm](https://helm.sh/docs/)
- [Kubernetes](https://kubernetes.io/docs/)
