package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type Installation struct {
	Path   string
	Addons []Addon
}

type Addon struct {
	Name       string
	Host       string
	Tar        string
	CurVersion string
}

var inst *Installation

func setup() {
	usr, err := user.Current()
	if err != nil {
		log.Fatalf("Unable to determine the current user: %v\n", err)
	}
	u := usr.HomeDir
	_, err = os.Stat(u)
	if err != nil {
		os.Mkdir(u, os.ModeDir)
	}
	l := filepath.Join(u, ".addons")
	_, err = os.Stat(l)
	if err != os.ErrNotExist {
		os.Mkdir(l, os.ModeDir)
	}
	l = filepath.Join(u, ".addons", "installed.json")
	_, err = os.Stat(l)
	if err != os.ErrNotExist {
		os.OpenFile(l, os.O_RDONLY|os.O_CREATE, 0666)
	}
	d, err := ioutil.ReadFile(l)
	if err != nil {
		log.Fatalf("Unable to perform setup: %v\n", err)
	}
	k := Installation{}
	if len(d) == 0 {
		k.Path = `C:\Program Files (x86)\World of Warcraft\Interface\AddOns`
	} else {
		err = json.Unmarshal(d, &k)
		if err != nil {
			log.Fatalf("Unable to read installation file: %v\n", err)
		}
	}
	inst = &k
}

/*
	Name: "curse",
	ProjectOverviewLocation: "https://wow.curseforge.com/projects/%v",
	ProjectIndexLocation:    "https://wow.curseforge.com/projects/%v/files",
	ReleaseDownloadLocation: "https://wow.curseforge.com/projects/%v/files/%v/download",

	Name: "wowace",
	ProjectOverviewLocation: "https://www.wowace.com/projects/%v",
	ProjectIndexLocation:    "https://www.wowace.com/projects/%v/files",
	ReleaseDownloadLocation: "https://www.wowace.com/projects/%v/files/%v/download",
*/

func main() {
	setup()

	switch os.Args[1] {
	case "install", "i":
		toInstall := os.Args[2:]
		var zips [][]byte
		m := sync.Mutex{}
		wg := sync.WaitGroup{}
		for _, a := range toInstall {
			var prov string
			var addon string
			x := strings.Split(a, ":")
			addon = x[0]
			if len(x) == 1 {
				prov = "curse"
			} else {
				prov = x[1]
			}

			var urlIndex string
			var urlDownload string
			switch prov {
			case "curse":
				urlIndex = "https://wow.curseforge.com/projects/%v/files"
				urlDownload = "https://wow.curseforge.com/projects/%v/files/%v/download"
			case "wowace":
				urlIndex = "https://www.wowace.com/projects/%v/files"
				urlDownload = "https://www.wowace.com/projects/%v/files/%v/download"
			default:
				log.Fatalf("Unknown download provider %v", prov)
			}

			wg.Add(1)
			go func() {
				r, err := http.Get(fmt.Sprintf(urlIndex, addon))
				if err != nil {
					log.Fatalf("Unable to download file: %v\n", err)
				}
				defer r.Body.Close()
				doc, err := goquery.NewDocumentFromReader(r.Body)
				if err != nil {
					log.Fatalf("Unable to parse the returned document into goquery: %v", err)
				}
				latest := 0
				items := doc.Find("table.project-file-listing tr.project-file-list-item")
				items.Each(func(i int, s *goquery.Selection) {
					//phase, _ := s.Find("td.project-file-release-type>div").Attr("class")
					href, _ := s.Find("div.project-file-download-button a.button.tip.fa-icon-download").Attr("href")
					x := strings.Split(href, "/")
					i, err := strconv.Atoi(x[len(x)-2])
					if err != nil {
						log.Fatalf("Unable to strconv %v\n", x[len(x)-2])
					}
					if i > latest {
						latest = i
					}
				})
				m.Lock()
				resp, err := http.Get(fmt.Sprintf(urlDownload, addon, latest))
				if err != nil {
					log.Fatalf("Unable to download file: %v", err)
				}
				d, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Fatalf("Unable to read file: %v", err)
				}
				defer resp.Body.Close()
				zips = append(zips, d)
				m.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()

		err := os.RemoveAll(inst.Path)
		if err != nil {
			log.Fatalf("Unable to clean directory: %v\n", err)
		}
		err = os.Mkdir(inst.Path, os.ModeDir)
		if err != nil {
			log.Fatalf("Unable to remake directory: %v\n", err)
		}
		var zipResults []*zip.File
		for _, z := range zips {
			z, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
			if err != nil {
				log.Fatalf("Unable to decode the zip: %v\n", err)
			}
			for _, f := range z.File {
				zipResults = append(zipResults, f)
			}
		}
		var dirs []string
		for _, f := range zipResults {
			dirs = append(dirs, filepath.Dir(f.Name))
		}
		sort.Strings(dirs)
		var strs []string
		i := -1
		for _, s := range dirs {
			if i < 0 || strs[i] != s {
				strs = append(strs, s)
				i++
			}
		}
		for _, d := range strs {
			os.MkdirAll(filepath.Join(inst.Path, d), os.ModeDir)
		}
		wg = sync.WaitGroup{}
		for _, f := range zipResults {
			wg.Add(1)
			go func(f *zip.File) {
				defer wg.Done()
				if f.FileInfo().IsDir() {
					return
				}
				r, err := f.Open()
				if err != nil {
					log.Fatalf("Unable to open file from zip: %v\n", err)
				}
				d, err := ioutil.ReadAll(r)
				if err != nil {
					log.Fatalf("Unable to read file from zip: %v\n", err)
				}
				err = ioutil.WriteFile(filepath.Join(inst.Path, f.Name), d, 0666)
				if err != nil {
					log.Fatalf("Unable to write file '%v': %v", f.Name, err)
				}
			}(f)
		}
		wg.Wait()
	}
}
