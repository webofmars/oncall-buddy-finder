FROM golang:1.8.3-alpine3.6

RUN apk add --no-cache git

WORKDIR /go/src/oncall-buddy-finder
COPY src/ .
COPY etc/ /etc/oncall-buddy-finder/

RUN go-wrapper download
RUN go-wrapper install
RUN mkdir /var/run/oncall-buddy-finder && chmod 755 /var/run/oncall-buddy-finder

CMD ["go-wrapper", "run"]

ENV CONFIG=/etc/oncall-buddy-finder/config-docker.json

VOLUME /etc/oncall-buddy-finder
VOLUME /var/run/oncall-buddy-finder
