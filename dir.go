package dirutil
import (
	"sort"
	"io/ioutil"
	"net/http"
	"net/url"
	"io"
	"os"
	"mime"
	"path"
	"fmt"
	"flag"
	"strings"
	"strconv"
	"text/template"
	"container/list"
	"compress/gzip"
	"compress/zlib"
	"time"
	"github.com/davidwalter1/tmplutil"
	// "golang.org/x/text/collate"
)
var fmap = template.FuncMap {
    "segue"     : tmplutil.Segue,
    "isMarkdown": tmplutil.IsMarkdown,
    "isWiki"    : tmplutil.IsWiki,
    "isHTML"    : tmplutil.IsHTML,
    "Cut"       : tmplutil.Cut,
    "Split"     : tmplutil.Split,
}				  

var RootFolder		   *string
var DirTemplate        *string = flag.String ( "dir-html-template",   "dir.html",	"header filename")
var UsesGzip  		   *bool   = flag.Bool	 ("gzip",             	  true,  		"Enables gzip/zlib compression")
var ListDirectories    *bool   = flag.Bool	 ("list-directories", 	  false, 		"list subdirectories, disabled by default")

const serverUA = "DW/0.0.1"
const fs_maxbufsize = 4096 // 4096 bits = default page size on OSX

/* Go is the first programming language with a templating engine embeddeed
 * but with no min function. */
func min(x int64, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

/*
func main() {
	// Get current working directory to get the file from it
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error while getting current directory.")
		return
	}

	// Command line parsing
	bind := flag.String("bind", ":1718", "Bind address")
	root_folder = flag.String("root", cwd, "Root folder")
	uses_gzip = flag.Bool("gzip", true, "Enables gzip/zlib compression")

	flag.Parse()

	http.Handle("/", http.HandlerFunc(handleFile))

	fmt.Printf("Sharing %s on %s ...\n", *root_folder, *bind)
	http.ListenAndServe((*bind), nil)
}
*/

// Manages directory listings
type dirlisting struct {
	Name           	   string
	Children_dir   	   []string
	Children_files 	   []string
	ServerUA       	   string
	ListSubDirectories bool
}

func copyToArray(src *list.List) []string {
	dst := make([]string, src.Len())

	i := 0
	for e := src.Front(); e != nil; e = e.Next() {
		dst[i] = e.Value.(string)
		i = i + 1
	}

	return dst
}

func DirectoryListing(w http.ResponseWriter, req *http.Request) {
	handleFile( w, req )
}

// From ioutil
// byName implements sort.Interface.
type FileByName []os.FileInfo

func (f FileByName) Len() int           { return len(f) }
func (f FileByName) Less(i, j int) bool { return f[i].Name() < f[j].Name() }
func (f FileByName) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type ByName []string

func (f ByName) Len() int           { return len(f) }
func (f ByName) Less(i, j int) bool { return f[i] < f[j] }
func (f ByName) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type ByNameIgnoreCase []string

func (f ByNameIgnoreCase) Len() int           { return len(f) }
func (f ByNameIgnoreCase) Less(i, j int) bool { return strings.ToLower( f[i] ) < strings.ToLower( f[j] ) }
func (f ByNameIgnoreCase) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

func handleDirectory(f *os.File, w http.ResponseWriter, req *http.Request) {
	names, _ := f.Readdir(-1)

	// First, check if there is any index in this folder.
	for _, val := range names {
		if val.Name() == "index.html" {
			serveFile(path.Join(f.Name(), "index.html"), w, req)
			return
		}
	}

	// Otherwise, generate folder content.
	children_dir_tmp := list.New()
	children_files_tmp := list.New()
	sort.Sort(FileByName(names))
	for _, val := range names {
		if val.Name()[0] == '.' {
			continue
		} // Remove hidden files from listing

		if val.IsDir() {
			children_dir_tmp.PushBack(val.Name())
		} else {
			children_files_tmp.PushBack(val.Name())
		}
	}

	// And transfer the content to the final array structure
	children_dir := copyToArray(children_dir_tmp)
	children_files := copyToArray(children_files_tmp)
	sort.Sort( ByNameIgnoreCase( children_files ) )
	if DirTemplate == nil {
		tmplutil.Error.Printf( "No file for directory html template specified" )
		os.Exit( 1 )
	}
	filename  := *DirTemplate
    text, err := ioutil.ReadFile( filename )
    if err != nil {
		tmplutil.Error.Printf( "File read error %s\n" , filename )
		os.Exit( 1 )
    }
	if text != nil { // dirlisting_tpl != nil {
		tpl, err := template.New("dir-template").Funcs( fmap ).Parse( string( text ) )
		if err != nil {
			http.Error(w, "500 Internal Error : Error while generating directory listing.", 500)
			fmt.Println(err)
			return
		}

		data := dirlisting{Name: req.URL.Path, ServerUA: serverUA, Children_dir: children_dir, Children_files: children_files, ListSubDirectories: *ListDirectories }

		err = tpl.Execute(w, data)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func serveFile(filepath string, w http.ResponseWriter, req *http.Request) {
	if tmplutil.IsHTML( filepath ) || tmplutil.IsWiki( filepath ) ||  tmplutil.IsMarkdown( filepath ) {
		tmplutil.Info.Printf( fmt.Sprintf( "Host %-20s Client %-20s URL.Path [%s] %s\n", 
			req.Host, req.RemoteAddr, req.URL.Path, tmplutil.LogString( req ) ) )
		if len( filepath ) > 0 && filepath[0] == '/' {
			filepath = filepath[1:]
		}
		text := tmplutil.Load( filepath )
		if text != nil {
			w.Header().Set("Content-Encoding", "text/plain")
			w.Write( []byte(tmplutil.WrapPre( filepath, *text )) )
			// w.Write( html.HTML( text ) )
		}
		return
	}

	tmplutil.Info.Printf( fmt.Sprintf( "Host %-20s Client %-20s URL.Path [%s] %s\n", 
		req.Host, req.RemoteAddr, req.URL.Path, tmplutil.LogString( req ) ) )
	// Opening the file handle
	f, err := os.Open(filepath)
	if err != nil {
		http.Error(w, "404 Not Found : Error while opening the file.", 404)
		return
	}

	defer f.Close()

	// Checking if the opened handle is really a file
	statinfo, err := f.Stat()
	if err != nil {
		http.Error(w, "500 Internal Error : stat() failure.", 500)
		return
	}

	if statinfo.IsDir() { // If it's a directory, open it !
		handleDirectory(f, w, req)
		return
	}

	if (statinfo.Mode() &^ 07777) == os.ModeSocket { // If it's a socket, forbid it !
		http.Error(w, "403 Forbidden : you can't access this resource.", 403)
		return
	}

	// Manages If-Modified-Since and add Last-Modified (taken from Golang code)
	if t, err := time.Parse(http.TimeFormat, req.Header.Get("If-Modified-Since")); err == nil && statinfo.ModTime().Unix() <= t.Unix() {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Last-Modified", statinfo.ModTime().Format(http.TimeFormat))

	// Content-Type handling
	query, err := url.ParseQuery(req.URL.RawQuery)
	tmplutil.Info.Printf( fmt.Sprintf( "Host %-20s Client %-20s URL.Path [%s] %s %s %s query %s\n", req.Host, req.RemoteAddr, req.URL.Path, tmplutil.LogString( req ), "filepath", filepath, query ) )

	if err == nil && len(query["dl"]) > 0 { // The user explicitedly wanted to download the file (Dropbox style!)
		w.Header().Set("Content-Type", "application/octet-stream")
	} else {
		// Fetching file's mimetype and giving it to the browser
		if mimetype := mime.TypeByExtension(path.Ext(filepath)); mimetype != "" {
			w.Header().Set("Content-Type", mimetype)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}

	// Manage Content-Range (TODO: Manage end byte and multiple Content-Range)
	if req.Header.Get("Range") != "" {
		start_byte := parseRange(req.Header.Get("Range"))

		if start_byte < statinfo.Size() {
			f.Seek(start_byte, 0)
		} else {
			start_byte = 0
		}

		w.Header().Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start_byte, statinfo.Size()-1, statinfo.Size()))
	}
	// Manage gzip/zlib compression
	output_writer := w.(io.Writer)

	is_compressed_reply := false

	if (* UsesGzip ) == true && req.Header.Get("Accept-Encoding") != "" {
		encodings := parseCSV(req.Header.Get("Accept-Encoding"))

		for _, val := range encodings {
			if val == "gzip" {
				w.Header().Set("Content-Encoding", "gzip")
				output_writer = gzip.NewWriter(w)

				is_compressed_reply = true

				break
			} else if val == "deflate" {
				w.Header().Set("Content-Encoding", "deflate")
				output_writer = zlib.NewWriter(w)

				is_compressed_reply = true

				break
			}
		}
	}

	if !is_compressed_reply {
		// Add Content-Length
		w.Header().Set("Content-Length", strconv.FormatInt(statinfo.Size(), 10))
	}

	// Stream data out !
	buf := make([]byte, min(fs_maxbufsize, statinfo.Size()))
	n := 0
	for err == nil {
		n, err = f.Read(buf)
		output_writer.Write(buf[0:n])
	}

	// Closes current compressors
	switch output_writer.(type) {
	case *gzip.Writer:
		output_writer.(*gzip.Writer).Close()
	case *zlib.Writer:
		output_writer.(*zlib.Writer).Close()
	}

	f.Close()
}

func handleFile(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Server", serverUA)
	filepath := path.Join((*RootFolder), path.Clean(req.URL.Path))
	tmplutil.Info.Printf( fmt.Sprintf( "Host %-20s Client %-20s URL.Path [%s] %s %s %s\n", 
		req.Host, req.RemoteAddr, req.URL.Path, tmplutil.LogString( req ), "filepath", filepath ) )
	serveFile(filepath, w, req)
	tmplutil.Info.Printf( fmt.Sprintf( "Host %-20s Client %-20s URL.Path [%s] %s %s %s\n", 
		req.Host, req.RemoteAddr, req.URL.Path, tmplutil.LogString( req ), "filepath", filepath ) )
}

func parseCSV(data string) []string {
	splitted := strings.SplitN(data, ",", -1)

	data_tmp := make([]string, len(splitted))

	for i, val := range splitted {
		data_tmp[i] = strings.TrimSpace(val)
	}

	return data_tmp
}


func parseRange(data string) int64 {
	stop := (int64)(0)
	part := 0
	for i := 0; i < len(data) && part < 2; i = i + 1 {
		if part == 0 { // part = 0 <=> equal isn't met.
			if data[i] == '=' {
				part = 1
			}

			continue
		}

		if part == 1 { // part = 1 <=> we've met the equal, parse beginning
			if data[i] == ',' || data[i] == '-' {
				part = 2 // part = 2 <=> OK DUDE.
			} else {
				if 48 <= data[i] && data[i] <= 57 { // If it's a digit ...
					// ... convert the char to integer and add it!
					stop = (stop * 10) + (((int64)(data[i])) - 48)
				} else {
					part = 2 // Parsing error! No error needed : 0 = from start.
				}
			}
		}
	}

	return stop
}

