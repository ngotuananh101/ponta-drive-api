FROM golang:alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0

WORKDIR /build
COPY . .
RUN go mod tidy
RUN go build --ldflags "-s -w -extldflags -static" -o main .

FROM alpine:latest

WORKDIR /www

COPY --from=builder /build/main /www/
COPY --from=builder /build/.env /www/.env
COPY --from=builder /build/app/ /www/app/
COPY --from=builder /build/bootstrap/ /www/bootstrap/
COPY --from=builder /build/config/ /www/config/
COPY --from=builder /build/database/ /www/database/
COPY --from=builder /build/lang/ /www/lang/
COPY --from=builder /build/public/ /www/public/
COPY --from=builder /build/resources/ /www/resources/
COPY --from=builder /build/routes/ /www/routes/
COPY --from=builder /build/tests/ /www/tests/
RUN mkdir -p /www/storage /www/tmp

ENTRYPOINT ["/www/main"]
