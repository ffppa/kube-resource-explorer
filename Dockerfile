FROM golang:1.23.2-alpine AS builder

ARG BASEDIR=/app/kube-resource-explorer/

RUN mkdir -p ${BASEDIR}
WORKDIR ${BASEDIR}

ENV CGO_ENABLED=0
ENV GOOS=linux

COPY . .
RUN apk add make && make build

FROM alpine:3.20.3

RUN apk add gcc py3-pip musl-dev python3-dev libffi-dev openssl-dev cargo make \
    && pip install --upgrade pip --break-system-packages \
    && pip install azure-cli --break-system-packages && az aks install-cli

COPY --from=builder /tmp /tmp
COPY --from=builder /app/kube-resource-explorer /

CMD ["/kube-resource-explorer"]
