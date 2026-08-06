FROM golang:1.26-alpine AS builder

WORKDIR /app

# proxy.golang.org 403s some module paths from this host's network; goproxy.cn is a full mirror
# that works here. Overridable via --build-arg GOPROXY=... on a network where the default works.
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server
# Self-contained (migrations compiled in via internal/repository's go:embed, not read from disk
# at runtime), so it works from this distroless image with no source checkout. Run as a one-off
# before rolling out a new version: `docker run --rm <image> /migrate`.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/server /server
COPY --from=builder /app/migrate /migrate

EXPOSE 8080

CMD ["/server"]
