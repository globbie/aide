FROM globbie/build as builder
ARG TRAVIS_BRANCH
ENV TRAVIS_BRANCH $TRAVIS_BRANCH

ENV D=$GOPATH/src/github.com/globbie/gnode
ADD . $D/
WORKDIR $D

RUN ./build_knowdy.sh

RUN dep ensure -v --vendor-only
RUN go get ./...
RUN go get golang.org/x/tools/cmd/cover
RUN go get github.com/mattn/goveralls

RUN echo " branch: " $TRAVIS_BRANCH
RUN go build -o gnode cmd/gnode/*.go
RUN go test -v -covermode=count -coverprofile=coverage.out ./...
RUN $GOPATH/bin/goveralls -coverprofile=coverage.out -v -service=travis-ci

RUN cp gnode /tmp/

# package stage
FROM alpine:latest
RUN apk add --no-cache libc6-compat

WORKDIR /etc/gnode/schemas
COPY ./examples /etc/gnode/

WORKDIR /etc/knowdy/schemas
COPY ./schemas /etc/knowdy/schemas

COPY --from=builder /tmp/gnode /usr/bin/

RUN adduser -D knowdy
WORKDIR /var/lib/knowdy/db
RUN chown -R knowdy /var/lib/knowdy
USER knowdy

EXPOSE 8080
CMD ["gnode", "--listen-address=0.0.0.0:8080", "--config-path=/etc/gnode/gnode.json"]
