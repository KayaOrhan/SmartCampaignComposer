FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY docs/embed.go docs/openapi.json ./docs/
COPY seed ./seed
RUN CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -o /worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -o /mockprovider ./cmd/mockprovider

FROM scratch
COPY --from=build /api /api
COPY --from=build /worker /worker
COPY --from=build /mockprovider /mockprovider
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
