package main

import (
	"os"
	"fmt"
	"strings"
	stdioutil "io/ioutil"
	"encoding/json"

	"github.com/k0kubun/pp"
	"github.com/gosimple/slug"
	"github.com/karrick/godirwalk"
	"gopkg.in/yaml.v3"
	"github.com/blevesearch/bleve"
	log "github.com/sirupsen/logrus"
	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/analysis/token/camelcase"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/web"
	"github.com/blevesearch/bleve/analysis/char/html"
	"github.com/onionltd/oniontree-tools/pkg/types/service"
	"github.com/spf13/pflag"
	"github.com/iancoleman/strcase"
)

var (
	debugMode = false
	publicKeys = false
	query string
	debug           bool
	help            bool
)


// Create a GORM-backend model
type Tag struct {
	Name string `gorm:"size:32;unique" json:"name" yaml:"name"`
}

type Service struct {
	Name        string       `json:"name" yaml:"name"`
	Alias     	string     	 `json:"alias" yaml:"alias"`
	Slug        string       `json:"slug,omitempty" yaml:"slug,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	URLs        []*URL       `json:"urls,omitempty" yaml:"urls,omitempty"`
	PublicKeys  []*PublicKey `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	Tags        []*Tag       `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type URL struct {
	Href      string `json:"href" yaml:"href"`
	Healthy   bool   `json:"healthy" yaml:"healthy"`
	ServiceID uint   `json:"-" yaml:"-"`
}

type PublicKey struct {
	UID         string `json:"id,omitempty" yaml:"id,omitempty"`
	UserID      string `json:"user_id,omitempty" yaml:"user_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Value       string `json:"value" yaml:"value"`
	ServiceID   uint   `json:"-" yaml:"-"`
}

func main() {

	pflag.BoolVarP(&debug, "debug", "d", false, "debug mode")
	pflag.BoolVarP(&help, "help", "h", false, "help info")
	pflag.Parse()
	if help {
		pflag.PrintDefaults()
		os.Exit(1)
	}

	args := pflag.Args()
	if len(args) == 0 {
		log.Fatal("no query passed as argument, eg ./onion-blevesearch [query]")
	}

	queryStr := strings.Join(args, " ")
	pp.Println(queryStr)

	enFieldMapping := bleve.NewTextFieldMapping()
	enFieldMapping.Analyzer = "en"

	kwFieldMapping := bleve.NewTextFieldMapping()
	kwFieldMapping.Analyzer = keyword.Name

	mapping := bleve.NewIndexMapping()

	//tokenizers
	resultAnalyser := "resultAnalyser"
	if err := mapping.AddCustomAnalyzer(resultAnalyser, map[string]interface{}{
		"type":          custom.Name,
		"char_filters":  []string{html.Name},
		"tokenizer":     web.Name,
		"token_filters": []string{camelcase.Name, lowercase.Name},
	}); err != nil {
		log.Fatal(err)
	}

	// field mapping types
	keywordContent := bleve.NewTextFieldMapping()
	keywordContent.Analyzer = resultAnalyser

	svcMapping := bleve.NewDocumentMapping()
	svcMapping.AddFieldMappingsAt("Name", keywordContent)
	svcMapping.AddFieldMappingsAt("Alias", keywordContent)
	svcMapping.AddFieldMappingsAt("Description", enFieldMapping)
	svcMapping.AddFieldMappingsAt("URLs.Href", kwFieldMapping)
	svcMapping.AddFieldMappingsAt("Tags.Name", keywordContent)

	// mapping := bleve.NewIndexMapping()
	mapping.DefaultMapping = svcMapping

	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		panic(err)
	}

	// wlak through yaml files in tagged directory
	entries := dirWalkServices("./data/oniontree/tagged")
	if debugMode {
		pp.Println(entries)
	}

	// Index blogs
	for _, e := range entries {		
		index.Index(e.Name, e)
	}

	// Query string
	query := bleve.NewQueryStringQuery(queryStr)

	// Simple text search
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"*"}
	searchResult, err := index.Search(searchRequest)
	if err != nil || searchResult.Total == 0 {
		fmt.Println("Not found")
		return
	}

	fmt.Println("============================================================")
	fmt.Println("Simple search result:")
	fmt.Println("============================================================\n")

	for i, hit := range searchResult.Hits {
		jsonstr, _ := json.Marshal(hit.Fields)
		fmt.Printf("Hit[%d]: %v %v\n", i, hit.ID, string(jsonstr))
	}

	// Facets search
	facet := bleve.NewFacetRequest("Tags.Name", 10)
	searchRequest.AddFacet("Tags.Name", facet)
	searchResult, err = index.Search(searchRequest)
	if err != nil || searchResult.Total == 0 {
		fmt.Println("Facets Not found")
		return
	}

	fmt.Println("============================================================")
	fmt.Println("Facets search result:")
	fmt.Println("============================================================\n")

	for i, hit := range searchResult.Hits {
		jsonstr, _ := json.Marshal(hit.Fields)
		fmt.Printf("Hit[%d]: %v %v\n", i, hit.ID, string(jsonstr))
	}

	for fname, fresult := range searchResult.Facets {
		jsonstr, _ := json.Marshal(fresult)
		fmt.Println("Facets:", fname, string(jsonstr))
		fmt.Println("Tags:")
		for _, tfacet := range fresult.Terms {
			fmt.Printf("\t%s (%d)\n", tfacet.Term, tfacet.Count)
		}
	}

}

func dirWalkServices(dirname string) (map[string]*Service) {
	entries := make(map[string]*Service, 0)
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

				slugName := slug.Make(t.Name)

				// add service
				m := &Service{
					Alias: 		 strcase.ToDelimited(t.Name, ' '),
					Name:        t.Name,
					Description: t.Description,
					Slug:        slugName,
				}
				// pp.Println(m)
				if entries[slugName] == nil { 
					entries[slugName] = m
				}

				// add public keys
				if publicKeys {
					for _, publicKey := range t.PublicKeys {
						pubKey := &PublicKey{
							UID:         publicKey.ID,
							UserID:      publicKey.UserID,
							Fingerprint: publicKey.Fingerprint,
							Description: publicKey.Description,
							Value:       publicKey.Value,
						}
						entries[slugName].PublicKeys = append(entries[slugName].PublicKeys, pubKey)
						// pp.Println(pubKey)
					}
				}

				// add urls
				for _, url := range t.URLs {
					u := &URL{Href: url}
					entries[slugName].URLs = append(entries[slugName].URLs, u)
					// pp.Println(u)
				}

				// add tags
				// check if tag already exists
				tag := &Tag{Name: parts[1]}
				entries[slugName].Tags = append(entries[slugName].Tags, tag)

				// entries = append(entries, m)
				// pp.Println(tag)
			}
			return nil
		},
		Unsorted: true, // (optional) set true for faster yet non-deterministic enumeration (see godoc)
	})
	if err != nil {
		log.Fatal(err)
	}
	return entries
}
