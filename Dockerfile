FROM golang:1.20

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app
RUN  CGO_ENABLED=0 GOOS=linux go build -o webhook-server .

# Use a minimal image for the final build
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the binary
COPY --from=0 /app/webhook-server .

# Expose the port
EXPOSE 8443

# Run the webhook server
CMD ["./webhook-server"]
