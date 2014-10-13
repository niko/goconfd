package main

// migrate lane_groove configuration:
// curl 'confserver:6666/conf.json' | json_reformat > conf.json

// curl post template examples:
// curl --data-binary @nginx.filedist.template confserver:9999/mp3_storage/iceman1
// mysql `curl --data-binary '--host={{.host}}:{{.port}} --user={{.username}} --password={{.password}}' confserver:9999/mysql`

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
	"os"
	"path"
	"net"
	"strings"
	"text/template"
)

func parseConfFile(path_components []string, conf_file_name string) (interface{}, error) {
	var i int
	var err error
	var key string
	var conf map[string]interface{}
	var conf_file_content []byte

	log.Printf("reading file %s with keys [%s]", conf_file_name, strings.Join(path_components, ", "))
	conf_file_content, err = ioutil.ReadFile(conf_file_name)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(conf_file_content, &conf)
	if err != nil {
		return nil, err
	}

	for i, key = range path_components {
		switch subtree := conf[key].(type) {
		case map[string]interface{}:
			conf = subtree
		case string:
			if i+1 < len(path_components) {
				return nil, errors.New("404")
			}
			return subtree, nil
		default:
			if i+1 < len(path_components) {
				return nil, errors.New("404")
			}
			if conf[key] == nil {
				return nil, errors.New("404")
			}
			return conf[key], nil
		}
	}
	return conf, nil
}

func toJson(conf interface{}) string {
	var json_bytes []byte

	switch conf := conf.(type) {
	case string:
		return string(conf)
	default:
		json_bytes, _ = json.MarshalIndent(conf, "", "  ")
		return string(json_bytes)
	}
}

func add(a, b int) int { return a + b }

func first(s []string) string { return s[0] }
func last(s []string) string { return s[len(s)-1] }

func now() (string) { return time.Now().Format("2006-01-02 15:04:05") }
func today() (string) { return time.Now().Format("2006-01-02") }

func join(if_ary []interface{}, sep string) (string) {
	var s_ary []string
	for _,e := range if_ary {
		if es, ok := e.(string); ok{
			s_ary = append(s_ary, es)
		}
	}
	return strings.Join(s_ary, sep)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"path_join": path.Join,
		"split":     strings.Split,
		"join":      join,
		"add":       add,
		"first":     first,
		"last":      last,
		"now":       now,
		"today":     today,
	}
}

func renderTemplate(w http.ResponseWriter, tmpl_string string, conf interface{}) {
	var err error
	var tmpl *template.Template
	var response bytes.Buffer

	tmpl, err = template.New("Post Body").Funcs(funcMap()).Parse(tmpl_string)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Error parsing template", 500)
		return
	}

	err = tmpl.Execute(&response, conf)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Error executing template", 500)
		return
	}

	fmt.Fprint(w, response.String())
}

var subscribers map[string]chan bool

func init() {
	subscribers = make(map[string]chan bool)
}
func waitFor(key string) {
	if _, ok := subscribers[key]; !ok {
		subscribers[key] = make(chan bool)
	}
	<-subscribers[key]
}
func trigger(key string) {
	if _, ok := subscribers[key]; ok {
		close(subscribers[key])
		delete(subscribers, key)
	}
}

func parseConfFileErrorHandler(w http.ResponseWriter, err error) {
	var json_bytes []byte

	log.Printf("Error: %s", err)

	switch err.Error() {
	case "404":
		w.WriteHeader(404)
		fmt.Fprint(w, "null")
		return
	default:
		w.WriteHeader(500)
		json_bytes, _ = json.Marshal(err.Error())
		fmt.Fprint(w, string(json_bytes))
		return
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	var conf_file_name, path_component string
	var conf interface{}
	var err error

	conf_file_name = flag.Arg(0)

	// sanitize path_components:
	var path_components []string
	for _, path_component = range strings.Split(r.URL.Path, "/") {
		if path_component != "" {
			path_components = append(path_components, path_component)
		}
	}

	switch strings.ToUpper(r.Method) {
	case "GET":
		if _, exists := r.URL.Query()["wait"]; exists {
			waitFor(r.URL.Path)
		}

		conf, err = parseConfFile(path_components, conf_file_name)
		if err != nil {
			parseConfFileErrorHandler(w, err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, toJson(conf))
	case "POST":
		if _, exists := r.URL.Query()["wait"]; exists {
			waitFor(r.URL.Path)
		}

		conf, err = parseConfFile(path_components, conf_file_name)
		if err != nil {
			parseConfFileErrorHandler(w, err)
		}

		body, _ := ioutil.ReadAll(r.Body)
		renderTemplate(w, string(body), conf)
	case "PUT":
		trigger(r.URL.Path)
	default:
		w.WriteHeader(402)
		fmt.Fprint(w, "Not supported")
	}
}

var (
	port = flag.Uint("port", 6666, "port")
)

var Usage = func() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s CONFFILE\n", os.Args[0])
	flag.PrintDefaults()
}

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, strings.ToUpper(r.Method), r.URL)
		handler.ServeHTTP(w, r)
	})
}

func JustLocal(handler http.Handler) http.Handler {
	var local_subnets []*net.IPNet
	local_subnet_s := []string{"127.0.0.1/31", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

	for _,net_s := range local_subnet_s {
		_, n, _ := net.ParseCIDR(net_s)
		local_subnets = append(local_subnets, n)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote_ip := net.ParseIP(strings.Split(r.RemoteAddr, ":")[0])

		local := false
		for _, local_subnet := range local_subnets {
			if local_subnet.Contains(remote_ip) {
				local = true
				break
			}
		}
		if !local {
			http.Error(w, "access denied", 403)
			return
		}
		handler.ServeHTTP(w, r)
		return
	})
}

func main() {
	flag.Usage = Usage
	flag.Parse()
	if flag.NArg() != 1 {
		Usage()
		os.Exit(1)
	}

	fmt.Println("Defined template helpers:")
	for k, _ := range funcMap() {
		fmt.Printf("%s(), ", k)
	}
	fmt.Println("")

	conf_file_name := flag.Arg(0)
	if _, err := os.Stat(conf_file_name); os.IsNotExist(err) {
		log.Printf("no such file or directory: %s\n", conf_file_name)
		os.Exit(1)
	}

	http.HandleFunc("/", handler)
	log.Printf("Listening on %d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), Log(JustLocal(http.DefaultServeMux))))
}
