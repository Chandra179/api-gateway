# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS build
ARG SERVICE=gateway
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# ---- runtime stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]