dep:
	go mod vendor

run:
	go run main.go

run_bindatafs:
	go run config/compile/compile.go
	go run -tags bindatafs main.go

build: dep
	go build main.go

build_bindatafs: dep
	go run config/compile/compile.go
	go build -tags bindatafs main.go
