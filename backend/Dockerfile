FROM golang:1.27.1-alpine3.23 AS base
WORKDIR /app
RUN apk add --no-cache git curl bash
COPY go.mod go.sum ./
RUN go mod download && go mod verify

FROM base AS dev
ARG AIR_VERSION=v1.67.3
RUN go install github.com/air-verse/air@${AIR_VERSION}
COPY . .
EXPOSE 8080
CMD ["air"]

FROM base AS build
COPY . .
RUN GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux go build -p 2 -trimpath -ldflags="-s -w" -o /out/bharatchat-server ./cmd/server

FROM base AS test
RUN apk add --no-cache gcc musl-dev
COPY . .
ENV CGO_ENABLED=1
CMD ["go", "test", "-race", "-count=1", "./..."]

FROM alpine:3.23 AS production
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S bharatchat \
    && adduser -S -G bharatchat -H bharatchat
WORKDIR /app
COPY --from=build --chown=bharatchat:bharatchat /out/bharatchat-server .
COPY --from=build --chown=bharatchat:bharatchat /app/migrations ./migrations
USER bharatchat
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["./bharatchat-server"]
