FROM golang:1.21 AS builder

RUN apt-get install -y make

COPY . /driver

RUN cd /driver && make build

FROM alpine:3.18

COPY --from=builder /driver/kontainer-engine-driver-lke /usr/bin
ENTRYPOINT ["kontainer-engine-driver-lke"]