# folder name of the package of interest
PKGNAME = remote
APPNAME = myapp
ARGS ?=
MKARGS ?= -timeout 120s

.PHONY: build run-server run-client final checkpoint all final-race checkpoint-race all-race clean docs
.SILENT: build run-server run-client final checkpoint all final-race checkpoint-race all-race clean docs

# build the server & client executables
build:
	go build -C $(APPNAME) $(APPNAME)_server.go
	go build -C $(APPNAME) $(APPNAME)_client.go

# run the server executable natively
run-server:
	go run -C $(APPNAME) $(APPNAME)_server.go $(ARGS)

# run the client executable natively
run-client:
	go run -C $(APPNAME) $(APPNAME)_client.go $(ARGS)

# run corresponding tests
final:
	go test -C $(PKGNAME) -v $(MKARGS) -run Final

checkpoint:
	go test -C $(PKGNAME) -v $(MKARGS) -run Checkpoint

all:
	go test -C $(PKGNAME) -v $(MKARGS)

# run corresponding tests using race detector
final-race:
	go test -C $(PKGNAME) -v $(MKARGS) -race -run Final

checkpoint-race:
	go test -C $(PKGNAME) -v $(MKARGS) -race -run Checkpoint

all-race:
	go test -C $(PKGNAME) -v $(MKARGS) -race

# delete all executables and docs, leaving only source
clean:
	rm -rf $(PKGNAME)/main $(PKGNAME)-doc.md $(APPNAME)/$(APPNAME)_server $(APPNAME)/$(APPNAME)_client

# generate documentation for the package of interest
docs:
	gomarkdoc -u -o $(PKGNAME)-doc.md ./$(PKGNAME)

