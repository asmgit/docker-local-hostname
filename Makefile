PROJECT     := github.com/asmgit/docker-local-hostname
VERSION     ?= v0.1.7
SETUP_IMAGE := ghcr.io/chipmk/docker-mac-net-connect/setup:v0.1.7
LDFLAGS     := -s -w -X $(PROJECT)/version.Version=$(VERSION) -X $(PROJECT)/version.SetupImage=$(SETUP_IMAGE)

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o docker-local-hostname .
