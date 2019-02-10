FROM golang as builder
RUN mkdir -p /go/src/github.com/rjacksonm1/time-is-money
ADD . /go/src/github.com/rjacksonm1/time-is-money
WORKDIR /go/src/github.com/rjacksonm1/time-is-money
RUN go get .
RUN go build -o /go/bin/time-is-money .

FROM golang:latest
COPY --from=builder /go/bin/time-is-money /app/
WORKDIR /app

RUN mkdir "/var/lib/time-is-money"
ENV DATA_DIR="/var/lib/time-is-money"
VOLUME "/var/lib/time-is-money"

CMD ["./time-is-money"]