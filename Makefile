# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
STATIC_FLAGS=-a -ldflags '-s -w -extldflags "-static"'

all: build-all-static

clean: 
	$(GOCLEAN)
	rm -f argo-nc

build-all-static: build-argo-nc-static

build-argo-nc-static:
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) $(STATIC_FLAGS) github.com/openxt/openxt-go/cmd/argo-nc
