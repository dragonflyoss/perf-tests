# # Copyright The Dragonfly Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Registry to push image-bench images to, e.g. D7Y_REGISTRY=ghcr.io/dragonflyoss make docker-push-image-bench.
D7Y_REGISTRY ?= dragonflyoss

all: help

# Build file-server image.
docker-build-file-server:
	@echo "Begin to use docker build file-server image."
	docker buildx build --platform linux/amd64,linux/arm64 -t file-server:latest -f ./tools/file-server/Dockerfile .
.PHONY: docker-build-file-server

# Build proxy-bench image.
docker-build-proxy-bench:
	@echo "Begin to use docker build proxy-bench image."
	docker buildx build --platform linux/amd64,linux/arm64 -t proxy-bench:latest -f ./tools/proxy-bench/Dockerfile .
.PHONY: docker-build-proxy-bench

# Build file-bench image.
docker-build-file-bench:
	@echo "Begin to use docker build file-bench image."
	docker buildx build --platform linux/amd64,linux/arm64 -t file-bench:latest -f ./tools/file-bench/Dockerfile .
.PHONY: docker-build-file-bench

# Build all image-bench images.
docker-build-image-bench: docker-build-image-bench-v1-1gb-4 docker-build-image-bench-v1-2gb-8 docker-build-image-bench-v1-4gb-8 docker-build-image-bench-v1-10gb-10 docker-build-image-bench-v1-20gb-4
	@echo "Build image-bench images done."
.PHONY: docker-build-image-bench

# Push all image-bench images.
docker-push-image-bench: docker-push-image-bench-v1-1gb-4 docker-push-image-bench-v1-2gb-8 docker-push-image-bench-v1-4gb-8 docker-push-image-bench-v1-10gb-10 docker-push-image-bench-v1-20gb-4
	@echo "Push image-bench images done."
.PHONY: docker-push-image-bench

# Build image-bench:v1-1gb-4 (1 GiB, 256 MiB x 4 layers).
docker-build-image-bench-v1-1gb-4:
	@echo "Begin to use docker build image-bench:v1-1gb-4 image."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(D7Y_REGISTRY)/image-bench:v1-1gb-4 -f ./build/images/image-bench/v1-1gb-4/Dockerfile .
.PHONY: docker-build-image-bench-v1-1gb-4

# Build image-bench:v1-2gb-8 (2 GiB, 256 MiB x 8 layers).
docker-build-image-bench-v1-2gb-8:
	@echo "Begin to use docker build image-bench:v1-2gb-8 image."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(D7Y_REGISTRY)/image-bench:v1-2gb-8 -f ./build/images/image-bench/v1-2gb-8/Dockerfile .
.PHONY: docker-build-image-bench-v1-2gb-8

# Build image-bench:v1-4gb-8 (4 GiB, 512 MiB x 8 layers).
docker-build-image-bench-v1-4gb-8:
	@echo "Begin to use docker build image-bench:v1-4gb-8 image."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(D7Y_REGISTRY)/image-bench:v1-4gb-8 -f ./build/images/image-bench/v1-4gb-8/Dockerfile .
.PHONY: docker-build-image-bench-v1-4gb-8

# Build image-bench:v1-10gb-10 (10 GiB, 1 GiB x 10 layers).
docker-build-image-bench-v1-10gb-10:
	@echo "Begin to use docker build image-bench:v1-10gb-10 image."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(D7Y_REGISTRY)/image-bench:v1-10gb-10 -f ./build/images/image-bench/v1-10gb-10/Dockerfile .
.PHONY: docker-build-image-bench-v1-10gb-10

# Build image-bench:v1-20gb-4 (20 GiB, 5 GiB x 4 layers).
docker-build-image-bench-v1-20gb-4:
	@echo "Begin to use docker build image-bench:v1-20gb-4 image."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(D7Y_REGISTRY)/image-bench:v1-20gb-4 -f ./build/images/image-bench/v1-20gb-4/Dockerfile .
.PHONY: docker-build-image-bench-v1-20gb-4

# Push image-bench:v1-1gb-4.
docker-push-image-bench-v1-1gb-4:
	@echo "Begin to push image-bench:v1-1gb-4 docker image."
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(D7Y_REGISTRY)/image-bench:v1-1gb-4 -f ./build/images/image-bench/v1-1gb-4/Dockerfile .
.PHONY: docker-push-image-bench-v1-1gb-4

# Push image-bench:v1-2gb-8.
docker-push-image-bench-v1-2gb-8:
	@echo "Begin to push image-bench:v1-2gb-8 docker image."
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(D7Y_REGISTRY)/image-bench:v1-2gb-8 -f ./build/images/image-bench/v1-2gb-8/Dockerfile .
.PHONY: docker-push-image-bench-v1-2gb-8

# Push image-bench:v1-4gb-8.
docker-push-image-bench-v1-4gb-8:
	@echo "Begin to push image-bench:v1-4gb-8 docker image."
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(D7Y_REGISTRY)/image-bench:v1-4gb-8 -f ./build/images/image-bench/v1-4gb-8/Dockerfile .
.PHONY: docker-push-image-bench-v1-4gb-8

# Push image-bench:v1-10gb-10.
docker-push-image-bench-v1-10gb-10:
	@echo "Begin to push image-bench:v1-10gb-10 docker image."
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(D7Y_REGISTRY)/image-bench:v1-10gb-10 -f ./build/images/image-bench/v1-10gb-10/Dockerfile .
.PHONY: docker-push-image-bench-v1-10gb-10

# Push image-bench:v1-20gb-4.
docker-push-image-bench-v1-20gb-4:
	@echo "Begin to push image-bench:v1-20gb-4 docker image."
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(D7Y_REGISTRY)/image-bench:v1-20gb-4 -f ./build/images/image-bench/v1-20gb-4/Dockerfile .
.PHONY: docker-push-image-bench-v1-20gb-4

# Run code lint
lint: markdownlint
	@echo "Begin to golangci-lint."
	@golangci-lint run
.PHONY: lint

# Run markdown lint
markdownlint:
	@echo "Begin to markdownlint."
	@./hack/markdownlint.sh
.PHONY: markdownlint

# Clear compiled files
clean:
	@go clean
	@rm -rf bin .go .cache
.PHONY: clean

help: 
	@echo "make docker-build-file-server               build file-server image"
	@echo "make docker-build-proxy-bench               build proxy-bench image"
	@echo "make docker-build-file-bench                build file-bench image"
	@echo "make docker-build-image-bench               build all image-bench images"
	@echo "make docker-build-image-bench-v1-1gb-4      build image-bench:v1-1gb-4 (1 GiB, 256 MiB x 4 layers)"
	@echo "make docker-build-image-bench-v1-2gb-8      build image-bench:v1-2gb-8 (2 GiB, 256 MiB x 8 layers)"
	@echo "make docker-build-image-bench-v1-4gb-8      build image-bench:v1-4gb-8 (4 GiB, 512 MiB x 8 layers)"
	@echo "make docker-build-image-bench-v1-10gb-10    build image-bench:v1-10gb-10 (10 GiB, 1 GiB x 10 layers)"
	@echo "make docker-build-image-bench-v1-20gb-4     build image-bench:v1-20gb-4 (20 GiB, 5 GiB x 4 layers)"
	@echo "make docker-push-image-bench                push all image-bench images"
	@echo "make docker-push-image-bench-v1-1gb-4       push image-bench:v1-1gb-4"
	@echo "make docker-push-image-bench-v1-2gb-8       push image-bench:v1-2gb-8"
	@echo "make docker-push-image-bench-v1-4gb-8       push image-bench:v1-4gb-8"
	@echo "make docker-push-image-bench-v1-10gb-10     push image-bench:v1-10gb-10"
	@echo "make docker-push-image-bench-v1-20gb-4      push image-bench:v1-20gb-4"
	@echo "make lint                                   run code lint"
	@echo "make markdownlint                           run markdown lint"
	@echo "make clean                                  clean"
