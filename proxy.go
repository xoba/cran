package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"
)

var packagesDir = "packages"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(os.Stderr, `
this http server functions as a special cran mirror —

  (1) works with a snapshot of the usually fluid PACKAGES.gz file.
  (2) transparently provides current or archived versions of a package's tar.gz.

request the uri "/install.r?packages=a,b,c,d,..." for an R install script
for packages a,b,c,d,... in sequence, with tests that packages were
actually installed.

`)
	}
	var mirror, packages string
	var port int
	var reload bool
	flag.StringVar(&mirror, "mirror", "cran.r-project.org", "the cran mirror's domain name")
	flag.StringVar(&packages, "packages", "latest", `the packages text file path ("latest" means last one in `+packagesDir+` dir)`)
	flag.IntVar(&port, "port", 80, "the port to run http server on; 0 chooses a random one")
	flag.BoolVar(&reload, "reload", false, "whether to load a new PACKAGES.gz from mirror")
	flag.Parse()
	PlatformInit()
	switch {
	case packages == "latest":
		dir, err := ioutil.ReadDir(packagesDir)
		check(err)
		var list []string
		for _, fi := range dir {
			list = append(list, filepath.Join(packagesDir, fi.Name()))
		}
		sort.Sort(sort.Reverse(sort.StringSlice(list)))
		packages = list[0]
	case reload:
		packages = time.Now().Format(filepath.Join(packagesDir, "20060102150405.txt"))
		os.MkdirAll(filepath.Dir(packages), os.ModePerm)
		u := fmt.Sprintf("http://%s/src/contrib/PACKAGES.gz", mirror)
		log.Printf("fetching %s --> %s", u, packages)
		resp, err := http.Get(u)
		if err != nil {
			check(err)
		}
		if resp.StatusCode != 200 {
			log.Fatal("bad http status: %s\n", resp.Status)
		}
		f, err := os.Create(packages)
		gz, err := gzip.NewReader(resp.Body)
		n, err := io.Copy(f, gz)
		check(err)
		log.Printf("wrote %d bytes to %s", n, packages)
		f.Close()
		resp.Body.Close()
	case len(packages) == 0:
		log.Fatal("needs a reference to a packages text file")
	}
	ln, err := newLocalListener(port)
	if err != nil {
		check(err)
	}
	self := fmt.Sprintf("http://%v", ln.Addr())
	log.Printf("running at %s with %s and mirror %s", self, packages, mirror)
	h := server{
		self:     self,
		packages: packages,
		mirror:   mirror,
	}
	s := &http.Server{
		Addr:    ":80",
		Handler: h,
	}
	check(s.Serve(ln))
}

func newLocalListener(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		ln, err = net.Listen("tcp6", fmt.Sprintf("[::1]:%d", port))
	}
	if err != nil {
		return nil, err
	}
	return ln, nil
}

type server struct {
	self     string
	packages string
	mirror   string
}

func (h server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	log.Printf("request from %s: %s %s", r.RemoteAddr, r.Method, r.RequestURI)
	fail := func(e error, code int) {
		log.Printf("** oops, failing for %s: %v", r.RequestURI, e)
		http.Error(w, e.Error(), code)
	}

	switch {

	case r.URL.Path == "/install.r":
		csv := r.URL.Query().Get("packages")
		if len(csv) == 0 {
			fail(fmt.Errorf("needs packages query parameter"), http.StatusBadRequest)
			return
		}
		var list []string
		for _, p := range strings.Split(csv, ",") {
			p = strings.TrimSpace(p)
			list = append(list, p)
		}

		t := template.Must(template.New("install.r").Parse(`# install.r generated at {{ .time }}
is.installed <- function(mypkg) is.element(mypkg, installed.packages()[,1])
mirror = "{{.self}}"
attempts = 10
pause = 3
{{ range .list }}
#
# package "{{ . }}"
#
if (!is.installed("{{.}}")) {
for (out in 1:attempts) {
install.packages("{{.}}",repo=mirror)
if (is.installed("{{.}}")) { break; } else { Sys.sleep(pause); }
}
if (!is.installed("{{.}}")) { quit(save='no',status=1) }
}
{{ end }}
`))
		check(t.Execute(w, map[string]interface{}{
			"self": h.self,
			"list": list,
			"time": time.Now().UTC(),
		}))

	case r.RequestURI == "/src/contrib/PACKAGES.gz":
		f, err := os.Open(h.packages)
		if err != nil {
			fail(err, http.StatusInternalServerError)
			return
		}
		defer f.Close()
		gz := gzip.NewWriter(w)
		defer gz.Close()
		_, err = io.Copy(gz, f)
		if err != nil {
			fail(err, http.StatusInternalServerError)
			return
		}

	case strings.HasPrefix(r.RequestURI, "/src/contrib"):
		file := filepath.Base(r.URL.Path)
		p, err := parsePackageFile(file)
		if err != nil {
			fail(err, http.StatusBadRequest)
			return
		}
		proxy := func(requestURI string) error {
			u := fmt.Sprintf("http://%s%s", h.mirror, requestURI)
			log.Printf("fetching %s", u)
			resp, err := http.Get(u)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			switch {
			case resp.StatusCode == 404:
				return fmt.Errorf("not found")
			case resp.StatusCode != 200:
				return fmt.Errorf("bad status: %s", resp.Status)
			}
			w.Header().Set("Content-Length", r.Header.Get("Content-Length"))
			_, err = io.Copy(w, resp.Body)
			return err
		}
		if err := proxy(r.RequestURI); err != nil {
			if err2 := proxy(fmt.Sprintf("/src/contrib/Archive/%s/%s", p.Name, file)); err2 != nil {
				fail(err2, http.StatusNotFound)
			}
		}

	default:
		http.NotFound(w, r)
	}
	return
}

type Package struct {
	File string
	Name string
}

func parsePackageFile(f string) (*Package, error) {
	parts := strings.Split(f, "_")
	if len(parts) < 2 {
		return nil, fmt.Errorf("needs '_'")
	}
	p := &Package{
		File: f,
		Name: parts[0],
	}
	return p, nil
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func PlatformInit() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
