GIT_COMMIT = $(shell git rev-parse HEAD)
GO_SOURCE_FILES = $(shell find pkg -type f -name "*.go")


build: $(GO_SOURCE_FILES)
	go mod tidy && \
	go build -ldflags \
		"-X main.GitCommit=${GIT_COMMIT} -extldflags '-static'" \
		-o kube-resource-explorer ./cmd/kube-resource-explorer

docker-build:
	docker build -t kube-resource-explorer .

run:
	docker run --rm -it \
		-v ${HOME}/.kube:/root/.kube \
		-v ${HOME}/.azure:/root/.azure \
		-v /etc/ssl/certs:/etc/ssl/certs \
		--network host \
		kube-resource-explorer
