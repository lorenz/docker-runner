FROM golang:1.13
ENV CGO_ENABLED 0
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build --ldflags="-w -s" -mod=readonly

FROM scratch
COPY --from=0 /build/docker-runner /docker-runner
CMD ["/docker-runner", "-logtostderr"]
