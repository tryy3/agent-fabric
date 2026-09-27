# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/controlplane ./cmd/controlplane \
 && mkdir -p /out/data \
 && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/controlplane /usr/local/bin/controlplane
COPY --from=build --chown=nonroot:nonroot /out/data /app/data
COPY --chown=nonroot:nonroot docker/sandbox.json /app/sandbox.json
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/controlplane"]
