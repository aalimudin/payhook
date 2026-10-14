FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /payhook ./cmd/payhook

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /payhook /payhook
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/payhook"]
