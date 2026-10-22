FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY seed ./seed
RUN CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api

FROM scratch
COPY --from=build /api /api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
