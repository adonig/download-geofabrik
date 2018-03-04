package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/proxy"

	"github.com/olekukonko/tablewriter"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

type config struct {
	BaseURL  string             `yaml:"baseURL"`
	Formats  map[string]format  `yaml:"formats,flow"`
	Elements map[string]element `yaml:"elements,flow"`
}

type element struct {
	ID     string   `yaml:"id"`
	File   string   `yaml:"file"`
	Meta   bool     `yaml:"meta"`
	Name   string   `yaml:"name"`
	Files  []string `yaml:"files"`
	Parent string   `yaml:"parent"`
}

type format struct {
	ID  string `yaml:"ext"`
	Loc string `yaml:"loc"`
}

var (
	app         = kingpin.New("download-geofabrik", "A command-line tool for downloading OSM files.")
	fConfig     = app.Flag("config", "Set Config file.").Default("./geofabrik.yml").Short('c').String()
	nodownload  = app.Flag("nodownload", "Do not download file (test only)").Short('n').Bool()
	verbose     = app.Flag("verbose", "Be verbose").Short('v').Bool()
	fProxyHTTP  = app.Flag("proxy-http", "Use http proxy, format: proxy_address:port").Default("").String()
	fProxySock5 = app.Flag("proxy-sock5", "Use Sock5 proxy, format: proxy_address:port").Default("").String()
	fProxyUser  = app.Flag("proxy-user", "Proxy user").Default("").String()
	fProxyPass  = app.Flag("proxy-pass", "Proxy password").Default("").String()

	update = app.Command("update", "Update geofabrik.yml from github")
	fURL   = update.Flag("url", "Url for config source").Default("https://raw.githubusercontent.com/julien-noblet/download-geofabrik/master/geofabrik.yml").String()

	list = app.Command("list", "Show elements available")
	lmd  = list.Flag("markdown", "generate list in Markdown format").Bool()

	download = app.Command("download", "Download element") //TODO : add d as command
	delement = download.Arg("element", "OSM element").Required().String()
	dosmBz2  = download.Flag("osm.bz2", "Download osm.bz2 if available").Short('B').Bool()
	dshpZip  = download.Flag("shp.zip", "Download shp.zip if available").Short('S').Bool()
	dosmPbf  = download.Flag("osm.pbf", "Download osm.pbf (default)").Short('P').Bool()
	doshPbf  = download.Flag("osh.pbf", "Download osh.pbf (default)").Short('H').Bool()
	dstate   = download.Flag("state", "Download state.txt file").Short('s').Bool()
	dpoly    = download.Flag("poly", "Download poly file").Short('p').Bool()
)

func (e *element) hasParent() bool {
	return len(e.Parent) != 0
}

func miniFormats(s []string) string {
	res := make([]string, 6)
	for _, item := range s {
		switch item {
		case "state":
			res[0] = "s"
		case "osm.pbf":
			res[1] = "P"
		case "osm.bz2":
			res[2] = "B"
		case "osh.pbf":
			res[3] = "H"
		case "poly":
			res[4] = "p"
		case "shp.zip":
			res[5] = "S"
		}
	}

	return strings.Join(res, "")
}

func downloadFromURL(myURL string, fileName string) {
	if *verbose {
		log.Println(" Downloading", myURL, "to", fileName)
	}

	if !*nodownload {
		// TODO: check file existence first with io.IsExist
		output, err := os.Create(fileName)
		if err != nil {
			log.Fatalln(fmt.Errorf(" Error while creating %s - %v", fileName, err))
			return
		}
		defer output.Close()
		transport := &http.Transport{}
		if *fProxyHTTP != "" {
			u, _ := url.Parse(myURL)
			//log.Println(u.Scheme +"://"+ *fProxyHTTP)
			proxyURL, err := url.Parse(u.Scheme + "://" + *fProxyHTTP)
			if *fProxyUser != "" && *fProxyPass != "" {
				proxyURL, err = url.Parse(u.Scheme + "://" + *fProxyUser + ":" + *fProxyPass + *fProxyHTTP)
			}
			if err != nil {
				log.Fatalln(fmt.Errorf(" Wrong proxy url, please use format proxy_address:port"))
				return
			}
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
		client := &http.Client{Transport: transport}
		if *fProxySock5 != "" {
			auth := proxy.Auth{User: *fProxyUser, Password: *fProxyPass}
			dialer, err := proxy.SOCKS5("tcp", *fProxySock5, &auth, proxy.Direct)
			if err != nil {
				log.Fatalln(fmt.Errorf(" Can't connect to the proxy: %v", err))
				return
			}
			transport.Dial = dialer.Dial
		}
		response, err := client.Get(myURL)
		if err != nil {
			log.Fatalln(fmt.Errorf(" Error while downloading %s - %v", myURL, err))
			return
		}
		defer response.Body.Close()

		n, err := io.Copy(output, response.Body)
		if err != nil {
			log.Fatalln(fmt.Errorf(" Error while downloading %s - %v", myURL, err))
			return
		}

		if *verbose {
			log.Println(" ", n, "bytes downloaded.")
		}
	}
}

func elem2preURL(c config, e element) string {
	var res string
	if e.hasParent() {
		res = elem2preURL(c, findElem(c, e.Parent)) + "/"
		if e.File != "" { //TODO use file in config???
			res = res + e.File
		} else {
			res = res + e.ID
		}
	} else {
		res = c.BaseURL + "/" + e.ID
	}
	return res
}

func elem2URL(c config, e element, ext string) string {
	res := elem2preURL(c, e)
	res += c.Formats[ext].Loc
	if !stringInSlice(ext, e.Files) {
		log.Fatalln(fmt.Errorf(" Error!!! %s not exist", res))
	}

	return res
}

func findElem(c config, e string) element {
	res := c.Elements[e]
	if res.ID == "" {
		log.Fatalln(fmt.Errorf(" %s is not in config\n Please use \"list\" command", e))
	}
	return res
}
func getFormats() []string {
	var formatFile []string
	if *dosmPbf {
		formatFile = append(formatFile, "osm.pbf")
	}
	if *doshPbf {
		formatFile = append(formatFile, "osh.pbf")
	}
	if *dosmBz2 {
		formatFile = append(formatFile, "osm.bz2")
	}
	if *dshpZip {
		formatFile = append(formatFile, "shp.zip")
	}
	if *dstate {
		formatFile = append(formatFile, "state")
	}
	if *dpoly {
		formatFile = append(formatFile, "poly")
	}
	if len(formatFile) == 0 {
		formatFile = append(formatFile, "osm.pbf")
	}
	return formatFile
}

func listAllRegions(c config, format string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeader([]string{"ShortName", "Is in", "Long Name", "formats"})
	if format == "Markdown" {
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")
	}
	keys := make(sort.StringSlice, len(c.Elements))
	i := 0
	for k := range c.Elements {
		keys[i] = k
		i++
	}
	keys.Sort()
	for _, item := range keys {
		table.Append([]string{item, c.Elements[c.Elements[item].Parent].Name, c.Elements[item].Name, miniFormats(c.Elements[item].Files)})
	}
	table.Render()
	fmt.Printf("Total elements: %#v\n", len(c.Elements))
}

func loadConfig(configFile string) config {
	filename, _ := filepath.Abs(configFile)
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalln(fmt.Errorf(" File error: %v ", err))
		os.Exit(1)
	}
	var myConfig config
	err = yaml.Unmarshal(file, &myConfig)
	if err != nil {
		panic(err)
	}
	return myConfig

}
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// UpdateConfig : simple script to download lastest config from repo
func UpdateConfig(myURL string, myconfig string) {
	downloadFromURL(myURL, myconfig)
	fmt.Println("Congratulation, you have the latest geofabrik.yml")
}

func main() {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {

	case list.FullCommand():
		var format = ""
		if *lmd {
			format = "Markdown"
		}
		listAllRegions(loadConfig(*fConfig), format)
	case update.FullCommand():
		UpdateConfig(*fURL, *fConfig)
	case download.FullCommand():
		formatFile := getFormats()
		for _, format := range formatFile {
			downloadFromURL(elem2URL(loadConfig(*fConfig), findElem(loadConfig(*fConfig), *delement), format), *delement+"."+format)
		}
	}
}
