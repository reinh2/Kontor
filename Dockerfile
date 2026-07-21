# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build

ARG TARGET=api
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kontor ./cmd/${TARGET}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kontor /kontor
USER nonroot:nonroot
ENTRYPOINT ["/kontor"]
