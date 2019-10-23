# syntax = docker/dockerfile:experimental
FROM golang:1.12 AS golang
WORKDIR /src
COPY . /go/src/github.com/ 
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /frontend --ldflags "-s -w" ./cmd/yarn-frontend/

FROM scratch AS release
COPY --from=golang /frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
