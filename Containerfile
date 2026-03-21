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
COPY . .
COPY --from=css /web/static/app.css web/static/app.css
RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o rconman ./cmd/rconman

FROM gcr.io/distroless/static-debian13
COPY --from=builder /app/rconman /rconman
COPY --from=builder /app/web/static /web/static
ENTRYPOINT ["/rconman"]
