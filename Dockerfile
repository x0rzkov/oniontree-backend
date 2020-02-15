FROM golang:1.13-alpine AS builder

RUN apk add --no-cache make gcc sqlite-dev sqlite musl-dev

COPY .  /go/src/github.com/onionltd/oniontree-backend
WORKDIR /go/src/github.com/onionltd/oniontree-backend

RUN cd /go/src/github.com/onionltd/oniontree-backend \
 && go install -v ./cmd/...

FROM alpine:3.11 AS runtime

# Install tini to /usr/local/sbin
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini-muslc-amd64 /usr/local/sbin/tini

# Install runtime dependencies & create runtime user
RUN apk --no-cache --no-progress add ca-certificates git libssh2 openssl \
	&& chmod +x /usr/local/sbin/tini && mkdir -p /opt \
	&& adduser -D onionltd -h /opt/oniontree -s /bin/sh \
	&& su onionltd -c 'cd /opt/oniontree; mkdir -p bin config data'

# Switch to user context
USER onionltd
WORKDIR /opt/oniontree

# Copy oniontree binary to /opt/oniontree/bin
COPY --from=builder /go/bin/oniontree-admin /opt/oniontree/bin/oniontree-admin
ENV PATH $PATH:/opt/oniontree/bin

# Container configuration
EXPOSE 9000
VOLUME ["/opt/oniontree/data"]
ENTRYPOINT ["tini", "-g", "--"]
CMD ["/opt/oniontree/bin/oniontree-admin"]
