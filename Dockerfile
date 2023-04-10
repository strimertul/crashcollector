FROM golang:alpine as golang

WORKDIR /go/src/app

COPY . .

# Static build required so that we can safely copy the binary over.
RUN CGO_ENABLED=0 go build -o /go/bin/app -ldflags '-extldflags "-static"' .

FROM alpine:latest as alpine

RUN apk --no-cache add tzdata zip ca-certificates

WORKDIR /usr/share/zoneinfo
# -0 means no compression.  Needed because go's
# tz loader doesn't handle compressed data.
RUN zip -r -0 /zoneinfo.zip .

FROM scratch

# copy app:
COPY --from=golang /go/bin/app /app

# copy timezone data:
ENV ZONEINFO /zoneinfo.zip
COPY --from=alpine /zoneinfo.zip /

# copy tls certificates:
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /
VOLUME /data

ENTRYPOINT ["/app"]