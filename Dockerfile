FROM golang:1.26-bookworm AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/c-mueller/ts-restic-server/internal/buildinfo.Version=${VERSION} -X github.com/c-mueller/ts-restic-server/internal/buildinfo.Commit=${COMMIT} -X github.com/c-mueller/ts-restic-server/internal/buildinfo.BuildDate=${BUILD_DATE}" \
    -o /ts-restic-server .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /ts-restic-server /usr/local/bin/ts-restic-server
EXPOSE 8880
ENTRYPOINT ["ts-restic-server"]
CMD ["serve"]
