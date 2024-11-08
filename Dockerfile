FROM golang:1.23.2-alpine AS builder

ARG BASEDIR=/app/kube-resource-explorer/

RUN mkdir -p ${BASEDIR}
WORKDIR ${BASEDIR}

ENV CGO_ENABLED=0
ENV GOOS=linux

COPY . .
RUN  apk add make && make build

FROM scratch
COPY --from=builder /tmp /tmp
COPY --from=builder /app/kube-resource-explorer /

ENTRYPOINT ["/kube-resource-explorer"]
