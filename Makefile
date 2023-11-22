KIND_VERSION = 0.20.0
KUBERNETES_VERSION = 1.27.3
KIND_NODE_HASH = 3966ac761ae0136263ffdb6cfd4db23ef8a83cba8a463690e98317add2c9ba72
CERT_MANAGER_VERSION = 1.13.2

GOLANGCI_LINT_VERSION=1.55.1
TAG  ?= $(shell git describe --tags --abbrev=0 HEAD || echo dev)
DATE_FMT = +"%Y-%m-%dT%H:%M:%S%z"
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif

GO_BUILD_VERSION_LDFLAGS=\
  -X go.szostok.io/version.version=$(TAG) \
  -X go.szostok.io/version.buildDate=$(BUILD_DATE) \
  -X go.szostok.io/version.commit=$(shell git rev-parse --short HEAD) \
  -X go.szostok.io/version.commitDate=$(shell git log -1 --date=format:"%Y-%m-%dT%H:%M:%S%z" --format=%cd) \
  -X go.szostok.io/version.dirtyBuild=false

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="$(GO_BUILD_VERSION_LDFLAGS)" -o dist/kibertas .

.PHONY: lint
lint:
	docker run --rm -v $(shell pwd):/app -v ~/.cache/golangci-lint/v$(GOLANGCI_LINT_VERSION):/root/.cache -w /app golangci/golangci-lint:v$(GOLANGCI_LINT_VERSION) golangci-lint run -v --timeout=5m

.PHONY: test
test:
	go test -timeout 4m -v ./...

.PHONY: goreleaser-snapshot
goreleaser-snapshot:
	curl -sfL https://goreleaser.com/static/run | bash -s -- --release --clean --snapshot

.PHONY: create-kind
create-kind:
	#@mkdir -p .bin/
	#@if [ ! -f " ./.bin/kind" ]; then \
	    curl -sSL -o ./.bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64; \
	    chmod +x ./.bin/kind; \
	fi
	#@sudo cp ./.bin/kind /usr/local/bin/kind;

	#@if [ ! -f "./.bin/kubectl" ]; then \
	#    curl -sSL -o ./.bin/kubectl https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl; \
	#    chmod +x ./.bin/kubectl; \
	#fi
	#@sudo cp ./.bin/kubectl /usr/local/bin/kubectl;
	kind create cluster --image kindest/node:v$(KUBERNETES_VERSION)@sha256:$(KIND_NODE_HASH) --wait 3m;

.PHONY: delete-kind
delete-kind:
	#@mkdir -p .bin/
	#@if [ ! -f " ./.bin/kind" ]; then \
	    curl -sSL -o ./.bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64; \
	    chmod +x ./.bin/kind; \
	fi
	#@sudo cp ./.bin/kind /usr/local/bin/kind;
	kind delete cluster

.PHONY: apply-cert-manager
apply-cert-manager:
	@kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
	@sleep 90
	@kubectl apply -f ./ClusterIssuer-SelfSigned.yaml

.PHONY: delete-cert-manager
delete-cert-manager:
	@kubectl delete -f ./ClusterIssuer-SelfSigned.yaml
	@kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
