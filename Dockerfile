FROM golang:1.24 AS builder

WORKDIR /

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o server ./cmd/server/main.go

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /server .

CMD ["./server"]