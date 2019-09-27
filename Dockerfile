FROM globbie/build as builder

ENV D=$GOPATH/src/github.com/globbie/gnode

ADD . $D/
WORKDIR $D

RUN dep ensure -v --vendor-only

RUN ./build_knowdy.sh

# todo: move it to the base image
RUN go get ./...
RUN go get github.com/mattn/goveralls
RUN go test -v -covermode=count -coverprofile=coverage.out ./...

RUN go build -o gnode cmd/gnode/*.go
RUN cp gnode /tmp/
RUN cp coverage.out /tmp/

# package stage
FROM alpine:latest
RUN apk add --no-cache libc6-compat

WORKDIR /etc/gnode/schemas
COPY ./examples /etc/gnode/
COPY ./schemas /etc/gnode/schemas

COPY --from=builder /tmp/gnode /usr/bin/

EXPOSE 8080
CMD ["gnode", "--listen-address=0.0.0.0:8080", "--config-path=/etc/gnode/gnode.json"]
