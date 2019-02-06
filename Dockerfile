FROM alpine:3.6 as alpine
RUN apk add -U --no-cache ca-certificates

FROM golang as builder
RUN mkdir -p /go/src/github.com/rjacksonm1/time-is-money
ADD . /go/src/github.com/rjacksonm1/time-is-money
WORKDIR /go/src/github.com/rjacksonm1/time-is-money
RUN go get .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o /go/bin/time-is-money .

FROM scratch
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/time-is-money /app/
WORKDIR /app
CMD ["./time-is-money"]