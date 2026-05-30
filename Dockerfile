# Build
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# Run
FROM alpine:3.19
WORKDIR /app
COPY --from=build /out/server /app/server
COPY internal/db/migrations /app/internal/db/migrations
COPY internal/dashboard/templates /app/internal/dashboard/templates
COPY internal/geo/data /app/internal/geo/data
COPY web /app/web
EXPOSE 8090
CMD ["/app/server"]
