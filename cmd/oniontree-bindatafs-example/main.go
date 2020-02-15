// based on https://doc.getqor.com/get_started.html
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/qor/admin"
	"github.com/qor/assetfs"
	"github.com/qor/qor/utils"

	_ "github.com/x0rzkov/oniontree-backend/pkg/bindatafs"
)

// Define a GORM-backend model
type User struct {
	gorm.Model
	Name string
}

// Define another GORM-backend model
type Product struct {
	gorm.Model
	Name        string
	Description string
}

func main() {
	// Set up the database
	DB, _ := gorm.Open("sqlite3", "demo.db")
	DB.AutoMigrate(&User{}, &Product{})

	// Initialize AssetFS
	AssetFS := assetfs.AssetFS().NameSpace("admin")
	// Register custom paths to manually saved views
	AssetFS.RegisterPath(filepath.Join(utils.AppRoot, "tmpl/qor/admin/views"))

	// Initialize Admin
	Admin := admin.New(&admin.AdminConfig{
		DB:      DB,
		AssetFS: AssetFS,
	})

	// Create resources from GORM-backend model
	Admin.AddResource(&User{})
	Admin.AddResource(&Product{})

	// Initialize an HTTP request multiplexer
	mux := http.NewServeMux()

	// Mount admin to the mux
	Admin.MountTo("/admin", mux)

	fmt.Println("Listening on: 9000")

	err := http.ListenAndServe(":9000", mux)
	if err != nil {
		log.Fatalln(err)
	}
}
