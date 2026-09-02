# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download

# Build a static binary. ops.json, templates and static assets are embedded via
# go:embed, so no runtime assets are needed. Prepare an empty data dir to copy.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/x-rest-api ./cmd/x-rest-api \
    && mkdir -p /out/data

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/x-rest-api /app/x-rest-api
COPY --from=build --chown=nonroot:nonroot /out/data /app/data

# SQLite lives here (holds plaintext cookies/keys); persist it with a volume.
ENV DB_PATH=/app/data/x-rest-api.db
VOLUME ["/app/data"]

EXPOSE 8430
USER nonroot:nonroot
ENTRYPOINT ["/app/x-rest-api"]
