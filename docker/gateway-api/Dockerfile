FROM golang:1.26.9-alpine

ENV AIR_VERSION=v1.63.0 \
    MIGRATE_VERSION=v4.18.1

RUN apk add --no-cache \
    git \
    make \
    curl \
    postgresql-client \
    netcat-openbsd

RUN go install github.com/air-verse/air@${AIR_VERSION}

RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}

WORKDIR /app

COPY gateway/go.mod gateway/go.sum ./
RUN go mod download

COPY gateway/ .
COPY docker/gateway-api/air.toml /docker/air.toml
COPY docker/gateway-api/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["air", "-c", "/docker/air.toml"]
