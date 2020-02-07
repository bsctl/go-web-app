FROM golang:1.13 as builder
WORKDIR /code
ADD go.mod go.sum /code/
RUN go mod download
ADD . .
RUN go build -o /app main.go

FROM gcr.io/distroless/base
ENV VERSION "v0.0.0"
EXPOSE 8080 9090
WORKDIR /
COPY --from=builder /app /usr/bin/app
ENTRYPOINT ["/usr/bin/app"]
