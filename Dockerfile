# Build Go Binary
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666 AS build

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
