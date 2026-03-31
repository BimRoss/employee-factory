FROM alpine:3.20

RUN apk add --no-cache bash ca-certificates curl git python3

ARG KUBECTL_VERSION=v1.31.0
RUN curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
  && chmod +x /usr/local/bin/kubectl

COPY scripts/persona-sync-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
