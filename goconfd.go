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
	"io"
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
		"trim":      strings.Trim,
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

	tmpl, err = template.New("Confserver Template").Funcs(funcMap()).Parse(tmpl_string)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), 500)
		return
	}

	err = tmpl.Execute(&response, conf)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprint(w, response.String())
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
	
	// GET plain JSON:
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
	
	// POST a golang template and have goconfd fill in the values:
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
	
	// Trigger blocking requests on this path:
	case "PUT":
		trigger(r.URL.Path)
	default:
		w.WriteHeader(402)
		fmt.Fprint(w, "Not supported")
	}
}

var (
	port = flag.Uint("port", 6666, "The port to run the conf server on.")
	redirect_to = flag.String("redirect-to", "", "The host and port of a master conf server where clients should be redirected to. E.g.: 10.0.0.30:6666")
	subscribers map[string]chan bool
)

func init() {
	subscribers = make(map[string]chan bool)
}

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

func fetchMasterConf(host string, conf_file string) {
	resp, err := http.Get("http://" + host + "?wait")
	if err != nil {
		fmt.Println("Couldn't get conf from master http://" + host)
		return
	}
	defer resp.Body.Close()

	backup_file := conf_file + "." + time.Now().Format("2006-01-02--15-04-05")

	out, err := os.Create(backup_file)
	if err == nil {
		fmt.Println("Saved master conf file as backup: " + backup_file)
	} else {
		fmt.Println("Couldn't save " + backup_file)
		return
	}
	defer out.Close()
	io.Copy(out, resp.Body)
}

func subscribeToMasterConf(host string, conf_file string) {
	for {
		fetchMasterConf(*redirect_to, conf_file)
		time.Sleep(10 * time.Second)
	}
}

func main() {
	flag.Usage = Usage
	flag.Parse()
	if flag.NArg() != 1 {
		Usage()
		os.Exit(1)
	}

	conf_file_name := flag.Arg(0)

	if *redirect_to == "" {
		fmt.Println("Defined template helpers:")
		for k, _ := range funcMap() {
			fmt.Printf("%s(), ", k)
		}
		fmt.Println("")

		if _, err := os.Stat(conf_file_name); os.IsNotExist(err) {
			log.Printf("no such file or directory: %s\n", conf_file_name)
			os.Exit(1)
		}

		http.HandleFunc("/", handler)
	} else {
		go subscribeToMasterConf(*redirect_to, conf_file_name)

		fmt.Println(*redirect_to)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
			http.Redirect(w, r, "http://" + *redirect_to + r.URL.RequestURI(), http.StatusTemporaryRedirect)
		})
	}


	log.Printf("Listening on %d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), Log(JustLocal(http.DefaultServeMux))))
}
