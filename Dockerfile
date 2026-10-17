# Build Go Binary
FROM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS build

WORKDIR /app
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY . .
RUN make build-binary

FROM debian:bullseye-slim

RUN apt-get -qqy update && \
    apt-get -qqy install git-core && \
    apt-get -qqy autoclean && \
    apt-get -qqy autoremove

COPY --from=build /app/kost /app/
ENTRYPOINT ["/app/kost"]
