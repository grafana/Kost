# Build Go Binary
FROM golang:1.23.2 AS build

WORKDIR /app
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY . .
RUN make build-binary

WORKDIR /root

COPY --from=build /app/kost ./
ENTRYPOINT ["./kost"]
