.PHONY: clean docker
all: clean install build docker

BIN=bin/
SRC=src/

VENDOR=vendor/

MAIN=oncall-buddy-finder.go
EXE=oncall-buddy-finder

clean: ## clean repo state
	go clean
	rm -rf bin/*
	rm -rf pkg/*
	rm -rf vendor/

install:
	go mod download
	go mod verify
	go mod tidy
	go mod vendor

build: install $(BIN)$(EXE) ## builds the app

$(BIN)$(EXE): $(SRC)$(MAIN)
	go build -o $@ $<

docker: ## builds the docker container
	docker build -t webofmars/oncall-buddy-finder:dev-rc .