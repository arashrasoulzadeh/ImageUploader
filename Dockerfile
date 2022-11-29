FROM golang:alpine

MAINTAINER Maintainer

ENV GIN_MODE=release
ENV PORT=8080
RUN apk add cmake make gcc libc-dev g++ openblas-dev libx11-dev
RUN apk add pkgconfig
RUN apk add vips

WORKDIR /go/src/github.com/ReviveDesignLab/upload_go

COPY . /go/src/github.com/ReviveDesignLab/upload_go

RUN go build github.com/ReviveDesignLab/upload_go

EXPOSE $PORT

RUN ls

ENTRYPOINT ["./app"]
