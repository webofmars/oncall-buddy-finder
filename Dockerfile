FROM golang:1.10 as builder

RUN export DEBIAN_FRONTEND=noninteractive && \
    apt-get update -y && apt-get upgrade -y && \
    apt-get -y install git && apt-get -y autoclean

COPY src/ /go/src/oncall-buddy-finder
WORKDIR /go/src/oncall-buddy-finder

RUN go get -v -d . && \
    CGO_ENABLED=0 GOOS=linux go build -i -v -a -installsuffix cgo -o oncall-buddy-finder . && ls -lR

FROM alpine:latest

LABEL maintainer="contact@webofmars.com"

RUN apk add --no-cache tzdata && \
    mkdir /var/run/oncall-buddy-finder && \
    chmod 777 /var/run/oncall-buddy-finder

COPY etc/ /etc/oncall-buddy-finder/
COPY --from=builder /go/src/oncall-buddy-finder/oncall-buddy-finder /usr/local/bin/

VOLUME /etc/oncall-buddy-finder
VOLUME /var/run/oncall-buddy-finder
WORKDIR /usr/local/bin
ENV CONFIG=/etc/oncall-buddy-finder/config-docker.json
USER nobody
CMD ["/usr/local/bin/oncall-buddy-finder"]