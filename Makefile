# Go parameters
GOCMD=go
GOMOD=$(GOCMD) mod
GOBUILD=$(GOCMD) build
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet

current_dir := $(dir $(abspath $(firstword $(MAKEFILE_LIST))))
GOBIN ?= $(current_dir)/bin

prefix = /usr
exec_prefix = $(prefix)
bindir = $(exec_prefix)/bin

PKGBASE := github.com/openxt/openxt-go
PKGS := argo db ioctl agent/provisioner installer
CMDS := argo-nc db-cmd dbus-send dbd prov-agent
VERSION := 0.1.0

# FIPS is not available until Go 1.24
#FIPS = GOFIPS140=v1.0.0

ifeq (1,${STATIC})
ENV = CGO_ENABLED=0 GOOS=linux ${FIPS}
FLAGS = -a -ldflags '-s -w -extldflags "-static"'
else
ENV = ${FIPS}
FLAGS = -ldflags="-s -w"
endif

ifeq (1,${ARGO})
TAGS = -tags argo
endif

all: ${CMDS}

clean: 
	@$(GOCLEAN)
	rm -rf $(GOBIN)

install: all
	@- $(foreach CMD, $(CMDS), \
		install -Dpm 0755 $(GOBIN)/$(CMD) $(DESTDIR)$(bindir)/$(CMD); \
	)

dep:
	$(GOMOD) download

vet:
	@- $(foreach PKG, $(PKGS), \
		$(GOVET) $(PKGBASE)/$(PKG); \
	)
	@- $(foreach CMD, $(CMDS), \
		$(GOVET) $(PKGBASE)/cmd/$(CMD); \
	)

gobin:
	[ -e "$(GOBIN)" ] || mkdir $(GOBIN)

pkgsite: gobin
	GOBIN=$(GOBIN) $(GOINSTALL) golang.org/x/pkgsite/cmd/pkgsite@latest


${CMDS}: gobin dep
	$(ENV) $(GOBUILD) $(TAGS) -o $(GOBIN)/$@ $(FLAGS) $(PKGBASE)/cmd/$@
