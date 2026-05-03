# Build Go Binary
FROM golang:1.25.1@sha256:d7098379b7da665ab25b99795465ec320b1ca9d4addb9f77409c4827dc904211 AS build

WORKDIR /app
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY . .
RUN make build-binary

FROM debian:bullseye-slim@sha256:4333240150a6924f878e05ec2c998aec95238010e0e4d2fec6161c90128c4652

RUN apt-get -qqy update && \
    apt-get -qqy install git-core && \
    apt-get -qqy autoclean && \
    apt-get -qqy autoremove

COPY --from=build /app/kost /app/
ENTRYPOINT ["/app/kost"]
