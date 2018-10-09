FROM golang:1.11
ENV CGO_ENABLED 0
WORKDIR $GOPATH/src/git.dolansoft.org/dolansoft/docker-runner
COPY . .
RUN go get . && go install --ldflags="-w -s" .

FROM scratch
COPY --from=0 /go/bin/docker-runner /docker-runner
CMD ["/docker-runner"]
