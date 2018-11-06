FROM globbie/build

ENV D=$GOPATH/src/github.com/globbie/gnode

ADD . $D/
WORKDIR $D

RUN dep ensure -v --vendor-only

RUN ls -ltr

RUN find . -name ".gitmodules" | xargs sed -i "s/git@github.com:/https:\/\/github.com\//"; \
        git submodule update --init; \
        find . -name ".gitmodules" | xargs sed -i "s/git@github.com:/https:\/\/github.com\//"; \
        git submodule update --init --recursive

RUN cd pkg/knowdy/knowdy && mkdir -p build && cd build && rm -rf * && cmake .. && make

RUN go test -v ./...
