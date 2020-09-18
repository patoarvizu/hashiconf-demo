FROM golang:1.13 as builder
COPY . /go/src/github.com/patoarvizu/hashiconf-demo/
WORKDIR /go/src/github.com/patoarvizu/hashiconf-demo/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /hashiconf-demo /go/src/github.com/patoarvizu/hashiconf-demo/cmd/main.go
FROM alpine:3.10.4
RUN apk update && apk add ca-certificates
COPY --from=builder /hashiconf-demo /
CMD /hashiconf-demo