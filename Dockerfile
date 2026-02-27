FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gtcp-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gt-agent ./cmd/agent

FROM alpine:3.20
RUN adduser -D -u 10001 gtcp
WORKDIR /app
COPY --from=build /out/gtcp-api /usr/local/bin/gtcp-api
COPY --from=build /out/gt-agent /usr/local/bin/gt-agent
EXPOSE 8080
USER gtcp
ENTRYPOINT ["/usr/local/bin/gtcp-api"]
