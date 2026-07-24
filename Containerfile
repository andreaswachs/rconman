FROM --platform=$BUILDPLATFORM node:22-alpine AS css
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
COPY internal/ /src/internal/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001
COPY . .
COPY --from=css /web/static/app.css web/static/app.css
RUN go generate ./...
RUN VERSION=$(grep '^tag:' image.yaml | awk -F'"' '{print $2}') && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-X github.com/your-org/rconman/internal/version.Version=$VERSION" \
    -o rconman ./cmd/rconman

FROM gcr.io/distroless/static-debian13
COPY --from=builder /app/rconman /rconman
ENTRYPOINT ["/rconman"]
