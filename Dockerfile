# Build Go Binary
FROM --platform=$BUILDPLATFORM golang:1.25.10 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY . .
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make build-binary

FROM debian:bookworm-slim

RUN apt-get -qqy update && \
    apt-get -qqy install git-core && \
    apt-get -qqy autoclean && \
    apt-get -qqy autoremove

COPY --from=build /app/kost /app/
ENTRYPOINT ["/app/kost"]
