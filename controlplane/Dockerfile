# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY controlplane/go.mod controlplane/go.sum ./
RUN go mod download
COPY controlplane/ ./
COPY LICENSE /licenses/AGPL-3.0.txt
COPY NOTICE /licenses/NOTICE.txt
COPY SOURCE.txt /licenses/SOURCE.txt
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    sh ./scripts/generate-third-party-notices.sh /licenses/THIRD_PARTY_NOTICES.txt \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/controlplane ./cmd/controlplane \
 && mkdir -p /out/data

# Alpine + docker-cli so the plane can exec `docker` against a mounted host socket.
# Distroless cannot run a host-mounted docker binary (dynamic linker / broken symlink).
FROM alpine:3.24
RUN apk add --no-cache ca-certificates docker-cli \
 && mkdir -p /app/data
WORKDIR /app
COPY --from=build /out/controlplane /usr/local/bin/controlplane
COPY --from=build /licenses /usr/share/licenses/agent-fabric
COPY controlplane/docker/config.json /app/config.json
# Root so /var/run/docker.sock (typically root:docker) works when mounted.
USER root
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/controlplane"]
