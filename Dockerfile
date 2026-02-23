FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ts-restic-server .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /ts-restic-server /usr/local/bin/ts-restic-server
EXPOSE 8880
ENTRYPOINT ["ts-restic-server"]
CMD ["serve"]
