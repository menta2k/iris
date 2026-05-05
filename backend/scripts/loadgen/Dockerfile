# syntax=docker/dockerfile:1.7
# Build context is the backend/ root so the loadgen can share go.mod / go.sum
# with the rest of the repo. Build via:
#   docker build -t iris/loadgen:dev -f scripts/loadgen/Dockerfile backend/

FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
        -o /out/loadgen ./scripts/loadgen

# distroless static keeps the image tiny; loadgen is pure-Go + stdlib net/smtp.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/loadgen /loadgen
USER nonroot:nonroot
ENTRYPOINT ["/loadgen"]
