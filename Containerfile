FROM --platform=$BUILDPLATFORM node:22-alpine AS css
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
# Copy templates so Tailwind can scan them for used classes
COPY internal/ /src/internal/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download
COPY . .
COPY --from=css /web/static/app.css web/static/app.css
RUN go generate ./...
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,id=go-build-$TARGETARCH \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o rconman ./cmd/rconman

FROM gcr.io/distroless/static-debian13
COPY --from=builder /app/rconman /rconman
ENTRYPOINT ["/rconman"]
