FROM golang:1.13 as builder
WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY main.go main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o webapp main.go

FROM gcr.io/distroless/static:nonroot
ENV VERSION "v0.0.0"
EXPOSE 8080 9090
WORKDIR /
COPY --from=builder /workspace/webapp .
USER nonroot:nonroot
ENTRYPOINT ["/webapp"]
