# qor_bindatafs_example

Steps:
1. basic setup with go modules, based on https://doc.getqor.com/get_started.html
2. follow bindatafs README.md https://github.com/qor/bindatafs:
   - install `GO111MODULE=off go get -u -f github.com/qor/bindatafs/...`  
     have to disable go modules, to have bindatafs be compiled to `$GOPATH/bin/bindatafs`.  
     bindatafs is a CLI;
   - initialize bindatafs with `bindatafs config/bindatafs`;
   - add `config/compile/compile.go`;  
     see https://github.com/qor/bindatafs#using-namespace.  
     some QOR modules are use assetfs namespaces;
   - import `config/bindatafs` to `main.go` for side effect purpose.
3. run/build with bindatafs:  
   first you need to execute `go run config/compile/compile.go` to compile registered paths into binary.
   after templates were successfully compiled `config/bindatafs/templates_bindatafs.go` file appears.  
   
`make build` - produces casual build  
`make build_bindatafs` - produces build with built in templates
