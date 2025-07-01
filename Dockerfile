FROM golang:1.24.4-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build for the target architecture with static linking
ARG TARGETARCH
ARG TARGETOS
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -extldflags=-static" \
    -trimpath \
    -o ditto ./main.go

FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /app/ditto ./ditto

ENTRYPOINT ["./ditto"]
