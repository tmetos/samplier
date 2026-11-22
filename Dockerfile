# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.Version=${VERSION}" \
      -o /out/samplier ./cmd/server

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/samplier /app/samplier

# Schema and seed are shipped for first-run/bootstrap tooling.
COPY schema.sql seed.sql /app/

USER nonroot:nonroot
EXPOSE 8080

ENV PORT=8080
ENTRYPOINT ["/app/samplier"]
CMD ["serve"]
