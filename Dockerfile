# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/seatkeyd ./cmd/seatkeyd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/seatkeyd /usr/local/bin/seatkeyd
ENV SEATKEY_DB_PATH=/data/seatkey.db
ENV SEATKEY_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/seatkeyd"]
