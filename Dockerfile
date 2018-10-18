FROM golang:1.11
ENV GO111MODULE on
ENV CGO_ENABLED 0
WORKDIR /build
COPY . .
RUN go build --ldflags="-w -s"

FROM scratch
COPY --from=0 /build/docker-runner /docker-runner
CMD ["/docker-runner"]
