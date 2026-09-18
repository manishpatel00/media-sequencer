# --- build stage -----------------------------------------------------------
FROM golang:1.22-bookworm AS build
WORKDIR /src

# mattn/go-sqlite3 uses cgo, so the build stage needs gcc (present in the
# non-slim "bookworm" base already). Copy go.mod/go.sum first so dependency
# downloads are cached across rebuilds that only change application code.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=1
RUN go build -o /out/server ./cmd/server

# --- runtime stage -----------------------------------------------------------
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/server ./server

# The SQLite file lives here; mount a volume at this path in production so
# data survives container restarts/redeploys.
ENV DB_PATH=/app/data/sequencer.db
ENV PORT=8080
EXPOSE 8080

VOLUME ["/app/data"]
ENTRYPOINT ["./server"]
