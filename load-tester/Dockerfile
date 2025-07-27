FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod init animal_shelter_load_tester
RUN go mod tidy
RUN go build -o load-tester .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/load-tester .

EXPOSE 3006
CMD ["./load-tester"]