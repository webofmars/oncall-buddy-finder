GOPATH := $(shell pwd)

.PHONY: clean docker
all: clean build docker

clean:
	go clean
	rm -rf bin/*
	rm -rf pkg/*
	rm -rf src/github.com/webofmars/oncall-buddy-finder/vendor/*

build:
	export GOPATH=${GOPATH} && \
	cd src/github.com/webofmars/oncall-buddy-finder && \
	dep ensure && \
	go build oncall-buddy-finder.go && \
	go install && \
	touch vendor/.gitkeep

docker:
	docker build -t webofmars/oncall-buddy-finder:dev-rc .