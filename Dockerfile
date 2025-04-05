# Start from the official Go image for building the app
FROM golang:1.23 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the rest of the source code
COPY . .

# Build the TCP server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o tcp-server .

# Final lightweight image
FROM alpine:latest

# Copy the compiled binary from the builder stage
COPY --from=builder /app/tcp-server /usr/local/bin/tcp-server

# Expose port 4567 for the TCP server
EXPOSE 4567

# Run the TCP server
CMD ["tcp-server", "server", "4567"]