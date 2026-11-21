# syntax=docker/dockerfile:1

# Base images are pinned by digest for reproducible builds; Dependabot
# (.github/dependabot.yml) proposes updates.
FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/go-crud .

# Distroless: no shell or package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/go-crud /go-crud
ENV GIN_MODE=release HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/go-crud"]
