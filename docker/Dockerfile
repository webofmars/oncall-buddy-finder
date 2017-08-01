FROM golang:1.8.3-alpine3.6

RUN apk add --no-cache git

WORKDIR /go/src/app
COPY src/ .
COPY etc/ /etc/oncall-buddy-finder/

RUN go-wrapper download
RUN go-wrapper install
RUN mkdir /root/.credentials && chmod 755 /root/.credentials

CMD ["go-wrapper", "run"]

ENV CONFIG=/etc/oncall-buddy-finder/config.json

VOLUME /etc/oncall-buddy-finder
VOLUME /root/.credentials
