FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags "-s -w" -o tarsmem0 .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/tarsmem0 .
EXPOSE 8080
CMD ["./tarsmem0"]
