FROM golang:1.21.0-alpine as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ditto ./main.go

FROM alpine:3.18
WORKDIR /app
RUN chown nobody:nobody /app
USER nobody:nobody
COPY --from=builder --chown=nobody:nobody ./app/ditto .
COPY --from=builder --chown=nobody:nobody ./app/run.sh .

ENTRYPOINT sh run.sh
