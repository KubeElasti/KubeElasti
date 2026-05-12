# Build the grpc-test-service binaries
# NOTE: golang:1.25.9 matches the project-wide Go toolchain version used in
# operator/Dockerfile and resolver/Dockerfile. If building outside CI, ensure
# this image is available in your registry or pin to a released version.
FROM golang:1.25.9 AS builder

WORKDIR /app

# Copy module files and download dependencies first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code (including pre-generated protobuf files)
COPY . .

# Build server and client binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /grpc-server ./server/
RUN CGO_ENABLED=0 GOOS=linux go build -o /grpc-client ./client/

# Use distroless as minimal base image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /grpc-server .
COPY --from=builder /grpc-client .
USER 65532:65532

ENTRYPOINT ["/grpc-server"]
