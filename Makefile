## run			:	run web interface.
.PHONY: run
run:
	@rm -f oniontree.db
	@go run main.go

dep:
	@GO111MODULE=off go get -u -f github.com/qor/bindatafs/...
	@go mod vendor

run_bindatafs:
	@go run ./config/compile/compile.go
	@go run -tags bindatafs ./cmd/oniontree-bindatafs/main.go

build: dep
	@go build main.go

build_bindatafs: dep
	@go run ./cmd/oniontree-compile/main.go
	@go build -tags bindatafs ./cmd/oniontree-bindatafs/main.go

## help			:	Print commands help.
.PHONY: help
help : Makefile
	@sed -n 's/^##//p' $<

# https://stackoverflow.com/a/6273809/1826109
%:
	@: