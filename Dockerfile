FROM gcr.io/distroless/static-debian12
#FROM amazon/aws-cli:latest

ARG TARGETOS
ARG TARGETARCH

COPY kibertas /usr/local/bin/kibertas

ENTRYPOINT ["/usr/local/bin/kibertas"]
