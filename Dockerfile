# --- 1. build the React frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- 2. build the Go backend (pure Go, no cgo) ---
FROM golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/mycab .

# --- 3. small runtime image ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D app
WORKDIR /app
COPY --from=api /out/mycab ./mycab
COPY --from=web /web/build ./static
ENV STATIC_DIR=/app/static
USER app
EXPOSE 8080
CMD ["./mycab"]
