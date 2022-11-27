FROM golang:alpine AS builder

# Set necessary environmet  variables needed for our image
ENV GO111MODULE=on \
    CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64
RUN apk add --no-cache ca-certificates build-base runc curl
# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o main .

# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/main .


# Build a small image
FROM alpine:latest
RUN apk add --no-cache ca-certificates runc curl

COPY --from=builder /dist/main /



# Command to run
ENTRYPOINT ["/main"]