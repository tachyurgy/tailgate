FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /fanoutd .
FROM scratch
COPY --from=build /fanoutd /fanoutd
EXPOSE 8080
ENTRYPOINT ["/fanoutd"]
