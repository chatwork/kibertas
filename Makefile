KIND_VERSION = 0.21.0
KUBERNETES_VERSION = 1.29.1
KIND_NODE_HASH = a0cc28af37cf39b019e2b448c54d1a3f789de32536cb5a5db61a49623e527144
CERT_MANAGER_VERSION = 1.14.1
INGRESS_NGINX_VERSION = $(shell grep "FROM registry.k8s.io/ingress-nginx/controller:" thirdparty/ingress-nginx/Dockerfile| cut -d':' -f2)

GOLANGCI_LINT_VERSION=1.59.0
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
	go test -timeout 6m -v ./util/... ./config/...

.PHONY: e2e/kindtest
e2e/kindtest:
	go test -timeout 20m -v ./cmd/...


# This will produce following images for testing locally:
# - examplecom/kibertas:canary-arm64
# - examplecom/kibertas:canary-amd64
.PHONY: goreleaser-snapshot
goreleaser-snapshot:
	curl -sfL https://goreleaser.com/static/run | REGISTRY=examplecom bash -s -- --clean --snapshot

.PHONY: create-kind
create-kind:
	kind create cluster --image kindest/node:v$(KUBERNETES_VERSION)@sha256:$(KIND_NODE_HASH) --wait 3m;

.PHONY: ci\:enable\:k8s
ci\:enable\:k8s:
	@mkdir -p .bin/
	@if [ ! -f " ./.bin/kind" ]; then \
	    curl -sSL -o ./.bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64; \
	    chmod +x ./.bin/kind; \
	fi
	@sudo cp ./.bin/kind /usr/local/bin/kind;

	@if [ ! -f "./.bin/kubectl" ]; then \
	    curl -sSL -o ./.bin/kubectl https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl; \
	    chmod +x ./.bin/kubectl; \
	fi
	@sudo cp ./.bin/kubectl /usr/local/bin/kubectl;
	kind create cluster --image kindest/node:v$(KUBERNETES_VERSION)@sha256:$(KIND_NODE_HASH) --wait 3m;

.PHONY: cloud-provider-kind
cloud-provider-kind:
	@go install sigs.k8s.io/cloud-provider-kind@latest
	@sudo install ~/go/bin/cloud-provider-kind /usr/local/bin

.PHONY: delete-kind
delete-kind:
	#@mkdir -p .bin/
	#@if [ ! -f " ./.bin/kind" ]; then \
	    curl -sSL -o ./.bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64; \
	    chmod +x ./.bin/kind; \
	fi
	#@sudo cp ./.bin/kind /usr/local/bin/kind;
	kind delete cluster

.PHONY: apply-ingress-nginx
apply-ingress-nginx:
	@kubectl apply --wait=true -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-${INGRESS_NGINX_VERSION}/deploy/static/provider/cloud/deploy.yaml
	@sleep 2
	@kubectl -n ingress-nginx wait deploy -l app.kubernetes.io/instance=ingress-nginx --for=condition=available --timeout=60s

.PHONY: delete-ingress-nginx
delete-ingress-nginx:
	@kubectl delete --wait=true -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-${INGRESS_NGINX_VERSION}/deploy/static/provider/cloud/deploy.yaml

.PHONY: apply-cert-manager
apply-cert-manager:
	@kubectl apply --wait=true -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
	@sleep 2
	@kubectl -n cert-manager wait deploy -l app.kubernetes.io/instance=cert-manager --for=condition=available --timeout=60s
	@kubectl apply -f ./manifests/ClusterIssuer-SelfSigned.yaml

.PHONY: delete-cert-manager
delete-cert-manager:
	@kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
