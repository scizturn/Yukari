FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go build -o /out/yukari ./cmd/yukari

FROM alpine:3.20

RUN addgroup -S yukari && adduser -S yukari -G yukari

WORKDIR /app
COPY --from=build /out/yukari /usr/local/bin/yukari
COPY data/sql ./data/sql

USER yukari

ENTRYPOINT ["yukari"]
