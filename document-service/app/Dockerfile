FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/app ./app

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/app /bin/app
EXPOSE 8090
CMD ["/bin/app"]
