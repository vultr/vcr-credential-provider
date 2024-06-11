VERSION ?= $VERSION
REGISTRY ?= $REGISTRY

.PHONY: deploy
deploy: clean docker-build docker-push

.PHONY: build
build:
	@echo "building vcr-credential-provider"
	go build -trimpath -o dist/vcr-credential-provider .

.PHONY: build-linux
build-linux:
	@echo "building vcr-credential-provider for linux"
	GOOS=linux GOARCH=amd64 GCO_ENABLED=0 go build  -trimpath -o dist/vcr-credential-provider ./cmd/provider.go

.PHONY: docker-build
docker-build:
	@echo "building docker image to dockerhub $(REGISTRY) with version $(VERSION)"
	docker build . -t $(REGISTRY)/vcr-credential-provider:$(VERSION)

.PHONY: docker-push
docker-push:
	docker push $(REGISTRY)/vcr-credential-provider:$(VERSION)

.PHONY: clean
clean:
	go clean -i -x ./...

.PHONY: test
test:
	go test -race github.com/vultr/vcr-credential-provider/pkg -v