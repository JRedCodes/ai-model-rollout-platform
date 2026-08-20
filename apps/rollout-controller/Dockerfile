# Self-contained Go module -- build context is this directory
# (apps/rollout-controller), not the repo root.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/rollout-controller .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/rollout-controller ./rollout-controller
COPY migrations ./migrations
EXPOSE 4003
ENTRYPOINT ["./rollout-controller"]
