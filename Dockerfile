FROM golang:1.21 AS builder

ENV CGO_ENABLED=0

WORKDIR /app

COPY go.mod /app
COPY go.sum /app

RUN go mod download

COPY . .

RUN go build -o kibertas .

FROM gcr.io/distroless/static-debian12
#FROM amazon/aws-cli:latest

ARG TARGETOS
ARG TARGETARCH

COPY --from=builder /app/kibertas /usr/local/bin/kibertas

ENTRYPOINT ["/usr/local/bin/kibertas"]
