# Build Go Binary
FROM golang:1.23.3 AS build

WORKDIR /app
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY . .
RUN make build-binary

WORKDIR /root

FROM debian:bullseye-slim

RUN apt-get -qqy update && \
    apt-get -qqy install git-core && \
    apt-get -qqy autoclean && \
    apt-get -qqy autoremove

COPY --from=build /app/kost ./
ENTRYPOINT ["./kost"]
