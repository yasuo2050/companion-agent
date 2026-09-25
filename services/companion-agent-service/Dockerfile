FROM golang:1.25-alpine AS build
WORKDIR /src
COPY contracts/companion-business ./contracts/companion-business
COPY services/companion-agent-service ./services/companion-agent-service
WORKDIR /src/services/companion-agent-service
RUN go mod download && CGO_ENABLED=0 go build -o /agent ./cmd/server
FROM alpine:3.22
RUN apk add --no-cache tzdata
RUN adduser -D -u 10001 app
USER app
COPY --from=build /agent /agent
WORKDIR /work
ENTRYPOINT ["/agent"]
