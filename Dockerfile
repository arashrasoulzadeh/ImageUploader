FROM golang:alpine

MAINTAINER Maintainer

ENV GIN_MODE=release
ENV PORT=8080

WORKDIR /go/src/github.com/ReviveDesignLab/upload_go

COPY . /go/src/github.com/ReviveDesignLab/upload_go

RUN go build github.com/ReviveDesignLab/upload_go

EXPOSE $PORT

RUN ls

ENTRYPOINT ["./app"]
