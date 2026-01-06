#syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.3
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=${TARGETARCH} go build \
        -trimpath \
        -o /bin/im-thread-service main.go

FROM alpine:3.20 AS final


LABEL org.opencontainers.image.title="IM Thread Service"
LABEL org.opencontainers.image.description="Webitel IM Thread Service"
LABEL org.opencontainers.image.vendor="Webitel"
LABEL org.opencontainers.image.source="https://github.com/webitel/im-thread-service"

RUN --mount=type=cache,target=/var/cache/apk \
    apk --update add \
        ca-certificates \
        tzdata \
        && \
        update-ca-certificates


ARG UID=10001
RUN adduser \
    -D \
    -g "" \
    -h "/nonexistent" \
    -s "/sbin/nologin" \
    -H \
    -u "${UID}" \
    webitel

USER webitel


COPY --from=build /bin/im-thread-service /bin/

ENTRYPOINT [ "/bin/im-thread-service" ]