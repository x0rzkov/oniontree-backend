package main

import (
	"bytes"
	"fmt"
	stdioutil "io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
	"github.com/k0kubun/pp"
	"github.com/karrick/godirwalk"
	_ "github.com/mattn/go-sqlite3"
	"github.com/qor/admin"
	"github.com/qor/qor/utils"
	"github.com/qor/assetfs"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/filemode"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
	"gopkg.in/src-d/go-git.v4/utils/ioutil"

	"github.com/onionltd/oniontree-tools/pkg/types/service"
	_ "github.com/x0rzkov/oniontree-backend/pkg/bindatafs"
)

var (
	debugMode  = true
	debugMode2 = true
	isTruncate = true
	db         *gorm.DB
	tables     = []interface{}{
		&Tag{},
		&Service{},
		&PublicKey{},
		&URL{},
	}
)

// Create a GORM-backend model
type Tag struct {
	gorm.Model
	Name string `gorm:"size:32;unique" json:"name" yaml:"name"`
}

type Service struct {
	gorm.Model
	Name        string       `json:"name" yaml:"name"`
	Slug        string       `json:"slug,omitempty" yaml:"slug,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	URLs        []*URL       `json:"urls,omitempty" yaml:"urls,omitempty"`
	PublicKeys  []*PublicKey `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	Tags        []*Tag       `gorm:"many2many:service_tags;" json:"tags,omitempty" yaml:"tags,omitempty"`
}

type URL struct {
	gorm.Model
	Name      string `gorm:"size:255;unique" json:"href" yaml:"href"`
	Healthy   bool   `json:"healthy" yaml:"healthy"`
	ServiceID uint   `json:"-" yaml:"-"`
}

type PublicKey struct {
	gorm.Model
	UID         string `gorm:"primary_key" json:"id,omitempty" yaml:"id,omitempty"`
	UserID      string `json:"user_id,omitempty" yaml:"user_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Value       string `json:"value" yaml:"value"`
	ServiceID   uint   `json:"-" yaml:"-"`
}

func main() {
	db, _ := gorm.Open("sqlite3", "oniontree.db")
	if isTruncate {
		truncateTables(db, tables...)
	}
	db.AutoMigrate(&Tag{}, &Service{}, &URL{}, &PublicKey{})
	if debugMode {
		db.LogMode(true)
	}

	// Initialize AssetFS
	AssetFS := assetfs.AssetFS().NameSpace("admin")

	// Register custom paths to manually saved views
	AssetFS.RegisterPath(filepath.Join(utils.AppRoot, "tmpl/qor/admin/views"))

	// Initalize
	// ref. https://doc.getqor.com/admin/general.html
	Admin := admin.New(&admin.AdminConfig{
		DB:       db,
		SiteName: "OnionTreeLtd",
		AssetFS:  AssetFS,
	})

	// Allow to use Admin to manage Tag, PublicKey, URL, Service
	Admin.AddResource(&Tag{})
	//Admin.AddResource(&Service{})

	svc := Admin.AddResource(&Service{})
	//	svc.IndexAttrs("Name", "URLs", "Tags")
	svc.Meta(&admin.Meta{
		Name: "Description",
		Type: "rich_editor",
	})
	//*/

	// Admin.AddResource(&PublicKey{})
	pks := Admin.AddResource(&PublicKey{})
	//		pks.IndexAttrs("UID", "UserID", "Description")
	pks.Meta(&admin.Meta{
		Name: "Value",
		Type: "text",
	})

	Admin.AddResource(&URL{})

	// getWorkTree(db)
	// os.Exit(1)
	dirWalkServices(db)

	// initalize an HTTP request multiplexer
	mux := http.NewServeMux()

	// Mount admin interface to mux
	Admin.MountTo("/admin", mux)

	fmt.Println("Listening on: 9000")
	http.ListenAndServe(":9000", mux)
}

func dirWalkServices(db *gorm.DB) {
	dirname := "./data/oniontree/tagged"
	err := godirwalk.Walk(dirname, &godirwalk.Options{
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if !de.IsDir() {
				parts := strings.Split(osPathname, "/")
				if debugMode {
					fmt.Printf("Type:%s osPathname:%s tag:%s\n", de.ModeType(), osPathname, parts[1])
				}
				bytes, err := stdioutil.ReadFile(osPathname)
				if err != nil {
					return err
				}
				t := service.Service{}
				yaml.Unmarshal(bytes, &t)
				if debugMode {
					pp.Println(t)
				}

				// add service
				m := &Service{
					Name:        t.Name,
					Description: t.Description,
					Slug:        slug.Make(t.Name),
				}

				if err := db.Create(m).Error; err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				// add public keys
				for _, publicKey := range t.PublicKeys {
					pubKey := &PublicKey{
						UID:         publicKey.ID,
						UserID:      publicKey.UserID,
						Fingerprint: publicKey.Fingerprint,
						Description: publicKey.Description,
						Value:       publicKey.Value,
					}
					if _, err := createOrUpdatePublicKey(db, m, pubKey); err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
				}

				// add urls
				for _, url := range t.URLs {
					var urlExists URL
					u := &URL{Name: url}
					if db.Where("name = ?", url).First(&urlExists).RecordNotFound() {
						db.Create(&u)
						if debugMode {
							pp.Println(u)
						}
					}
					if _, err := createOrUpdateURL(db, m, u); err != nil {
						fmt.Println(err)
						os.Exit(1)
					}

				}

				// add tags
				// check if tag already exists
				tag := &Tag{Name: parts[1]}
				var tagExists Tag
				if db.Where("name = ?", parts[1]).First(&tagExists).RecordNotFound() {
					db.Create(&tag)
					if debugMode {
						pp.Println(tag)
					}
				}

				if _, err := createOrUpdateTag(db, m, tag); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

			}
			return nil
		},
		Unsorted: true, // (optional) set true for faster yet non-deterministic enumeration (see godoc)
	})
	if err != nil {
		log.Fatal(err)
	}
}

func createOrUpdateTag(db *gorm.DB, svc *Service, tag *Tag) (bool, error) {
	var existingSvc Service
	if db.Where("slug = ?", svc.Slug).First(&existingSvc).RecordNotFound() {
		err := db.Create(svc).Error
		return err == nil, err
	}
	var existingTag Tag
	if db.Where("name = ?", tag.Name).First(&existingTag).RecordNotFound() {
		err := db.Create(tag).Error
		return err == nil, err
	}
	svc.ID = existingSvc.ID
	svc.CreatedAt = existingSvc.CreatedAt
	svc.Tags = append(svc.Tags, &existingTag)
	return false, db.Save(svc).Error
}

func findPublicKeyByUID(db *gorm.DB, uid string) *PublicKey {
	pubKey := &PublicKey{}
	if err := db.Where(&PublicKey{UID: uid}).First(pubKey).Error; err != nil {
		log.Fatalf("can't find public_key with uid = %q, got err %v", uid, err)
	}
	return pubKey
}

func createOrUpdatePublicKey(db *gorm.DB, svc *Service, pubKey *PublicKey) (bool, error) {
	var existingSvc Service
	if db.Where("slug = ?", svc.Slug).First(&existingSvc).RecordNotFound() {
		err := db.Create(svc).Error
		return err == nil, err
	}
	var existingPublicKey PublicKey
	if db.Where("uid = ?", pubKey.UID).First(&existingPublicKey).RecordNotFound() {
		err := db.Create(pubKey).Error
		return err == nil, err
	}
	svc.ID = existingSvc.ID
	svc.CreatedAt = existingSvc.CreatedAt
	svc.PublicKeys = append(svc.PublicKeys, &existingPublicKey)
	return false, db.Save(svc).Error
}

func createOrUpdateURL(db *gorm.DB, svc *Service, url *URL) (bool, error) {
	var existingSvc Service
	if db.Where("slug = ?", svc.Slug).First(&existingSvc).RecordNotFound() {
		err := db.Create(svc).Error
		return err == nil, err
	}
	var existingURL URL
	if db.Where("name = ?", url.Name).First(&existingURL).RecordNotFound() {
		err := db.Create(url).Error
		return err == nil, err
	}
	svc.ID = existingSvc.ID
	svc.CreatedAt = existingSvc.CreatedAt
	svc.URLs = append(svc.URLs, &existingURL)
	return false, db.Save(svc).Error
}

/*
// findServiceByTag finds a service by remote ID and service
func findServiceByTag(db *gorm.DB, remoteID string, service *Service) (*Star, error) {
	// Get existing by remote ID and service ID
	var star Star
	if db.Where("remote_id = ? AND service_id = ?", remoteID, service.ID).First(&star).RecordNotFound() {
		return nil, errors.New("not found")
	}
	return &star, nil
}
*/

func truncateTables(db *gorm.DB, tables ...interface{}) {
	for _, table := range tables {
		if debugMode {
			pp.Println(table)
		}
		if err := db.DropTableIfExists(table).Error; err != nil {
			panic(err)
		}
		db.AutoMigrate(table)
	}
}

// Clone a repository to memory and ...
func getWorkTree(db *gorm.DB) error {
	fs := memfs.New()
	r, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL:           "https://github.com/onionltd/oniontree",
		ReferenceName: plumbing.NewBranchReferenceName("master"),
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
	if err != nil {
		return err
	}

	head, err := r.Head()
	if err != nil {
		return err
	}

	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		return err
	}

	tree, err := r.TreeObject(commit.TreeHash)
	if err != nil {
		return err
	}

	type treePath struct {
		*object.Tree
		Path string
	}

	for frontier := []treePath{{Tree: tree, Path: "/"}}; len(frontier) > 0; frontier = frontier[1:] {
		t := frontier[0]

		for _, e := range t.Entries {
			pp.Println("e.Name: ", e.Name, "t.Path: ", t.Path)
			if e.Mode != filemode.Dir {
				// We only care about directories.
				continue
			}
			if strings.HasPrefix(e.Name, ".") || strings.HasPrefix(e.Name, "_") || e.Name == "testdata" {
				fmt.Println("Continue because e.Name=", e.Name)
				continue
			}
			tree, err := r.TreeObject(e.Hash)
			if err != nil {
				fmt.Println("error with e.Hash=", e.Hash)
				return err
			}
			frontier = append(frontier, treePath{
				Tree: tree,
				Path: path.Join(t.Path, e.Name),
			})
		}
		i := 0
		for _, e := range t.Entries {
			// if e.Mode != filemode.Regular && e.Mode != filemode.Executable {
			// fmt.Println("continue with filemode.Regular=", filemode.Regular, " or filemode.Executable", filemode.Executable)
			// continue
			// }

			parts := strings.Split(t.Path, "/")
			if len(parts) <= 2 {
				continue
			}

			if debugMode2 {
				fmt.Printf("Name:%s Path:%s Length parts:%d\n", e.Name, t.Path, len(parts))
				fmt.Printf("Name:%s Path:%s Tag:%s\n", e.Name, t.Path, parts[2])
				pp.Println("parts", parts)
				if i == 100 {
					os.Exit(1)
				}
				i++
			}

			fmt.Println(path.Join(t.Path, e.Name))

			blob, err := r.BlobObject(e.Hash)
			if err != nil {
				return err
			}

			r, err := blob.Reader()
			if err != nil {
				return err
			}

			svc := service.Service{}
			buf := new(bytes.Buffer)
			buf.ReadFrom(r)

			pp.Println("e.Mode: ", e.Mode)

			if e.Mode == filemode.Symlink {
				// pp.Println(e)
				oldname, err := fs.Readlink(path.Join(t.Path, e.Name))
				//err := checkoutFileSymlink(e.File, fs)
				if err != nil {
					return err
				}
				pp.Println("oldname: ", oldname)
				os.Exit(1)
				/*
				fi, err := fs.Lstat(path.Join(t.Path, e.Name))
				if err != nil {
					return err
				}
				*/
				// fs.Readlink
				/*
				pp.Println("fi: ", fi)
				err = checkoutFileSymlink(e.File, fs)
				if err != nil {
					return err
				}
				*/
			}

			// pp.Println("newStr", newStr)
			yaml.Unmarshal(buf.Bytes(), &svc)
			if debugMode {
				pp.Println("svc: ", svc)
			}

			if debugMode2 {
				pp.Println("===============================================================\n", buf.String())
			}
			r.Close()

			// add service
			m := &Service{
				Name:        svc.Name,
				Description: svc.Description,
				Slug:        slug.Make(svc.Name),
			}

			if err := db.Create(m).Error; err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			// add public keys
			for _, publicKey := range svc.PublicKeys {
				pubKey := &PublicKey{
					UID:         publicKey.ID,
					UserID:      publicKey.UserID,
					Fingerprint: publicKey.Fingerprint,
					Description: publicKey.Description,
					Value:       publicKey.Value,
				}
				if _, err := createOrUpdatePublicKey(db, m, pubKey); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			}

			// add urls
			for _, url := range svc.URLs {
				var urlExists URL
				u := &URL{Name: url}
				if db.Where("name = ?", url).First(&urlExists).RecordNotFound() {
					db.Create(&u)
					if debugMode {
						pp.Println(u)
					}
				}
				if _, err := createOrUpdateURL(db, m, u); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

			}

			// add tags
			// check if tag already exists
			tag := &Tag{Name: parts[2]}
			var tagExists Tag
			if db.Where("name = ?", parts[1]).First(&tagExists).RecordNotFound() {
				db.Create(&tag)
				if debugMode {
					pp.Println(tag)
				}
			}

			if _, err := createOrUpdateTag(db, m, tag); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	}

	return nil
}

func checkoutFileSymlink(f *object.File, fs billy.Filesystem) (err error) {
	from, err := f.Reader()
	if err != nil {
		return
	}
	defer ioutil.CheckClose(from, &err)
	bytes, err := stdioutil.ReadAll(from)
	if err != nil {
		return
	}
	err = fs.Symlink(string(bytes), f.Name)
	return
}
