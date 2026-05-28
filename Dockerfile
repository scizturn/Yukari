FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go build -o /out/yukari ./cmd/yukari
RUN go build -o /out/forcejob ./cmd/forcejob
RUN go build -o /out/migrateemailaudit ./cmd/migrateemailaudit

FROM alpine:3.20

RUN addgroup -S yukari && adduser -S yukari -G yukari

WORKDIR /app
COPY --from=build /out/yukari /usr/local/bin/yukari
COPY --from=build /out/forcejob /usr/local/bin/forcejob
COPY --from=build /out/migrateemailaudit /usr/local/bin/migrateemailaudit
COPY data/sql ./data/sql
COPY data/vouchers ./data/vouchers
COPY db/migrations ./db/migrations

USER yukari

CMD ["sh", "-c", "sleep infinity"]
