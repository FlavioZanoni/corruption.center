FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o corruption-api .

FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/corruption-api .

EXPOSE 8080

CMD ["./corruption-api"]
