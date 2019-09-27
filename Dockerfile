FROM globbie/build as builder

ENV D=$GOPATH/src/github.com/globbie/gnode

ADD . $D/
WORKDIR $D

RUN dep ensure -v --vendor-only

RUN ./build_knowdy.sh

RUN go get ./...
RUN go build -o gnode cmd/gnode/*.go
RUN go test -v -covermode=count -coverprofile=coverage.out ./...

RUN cp gnode /tmp/
RUN cp coverage.out /tmp/

# package stage
FROM alpine:latest
RUN apk add --no-cache libc6-compat

WORKDIR /etc/gnode/schemas
COPY ./examples /etc/gnode/
COPY ./schemas /etc/gnode/schemas

COPY --from=builder /tmp/gnode /usr/bin/
COPY --from=builder /tmp/coverage.out /tmp

EXPOSE 8080
CMD ["gnode", "--listen-address=0.0.0.0:8080", "--config-path=/etc/gnode/gnode.json"]
