FROM golang:1.25-alpine AS build
WORKDIR /src
COPY contracts/companion-business ./contracts/companion-business
COPY services/mock-business-service ./services/mock-business-service
WORKDIR /src/services/mock-business-service
RUN go mod download && CGO_ENABLED=0 go build -o /business ./cmd/server && CGO_ENABLED=0 go build -o /seed ./cmd/seed
FROM alpine:3.22
RUN apk add --no-cache tzdata
COPY --from=build /business /business
COPY --from=build /seed /seed
ENTRYPOINT ["/business"]
