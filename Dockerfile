FROM golang:1.22.3 as builder

WORKDIR /app

COPY ["go.mod", "go.sum", "./"]

RUN ["go", "mod", "download"]

COPY [".", "."]

RUN ["env", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64", "go", "build", "-o", "k8s-cost-estimator", "./cmd/bot"]

FROM debian:bullseye-slim

RUN apt-get -qqy update && \
    apt-get -qqy install git-core && \
    apt-get -qqy autoclean && \
    apt-get -qqy autoremove

COPY --from=builder /app/k8s-cost-estimator /app/

ENTRYPOINT ["/app/k8s-cost-estimator"]