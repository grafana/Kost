# Build Go Binary
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build

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
