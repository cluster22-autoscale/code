package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/iwqos22-autoscale/code/mock/utils"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
)

var (
	configFile string
	addr string
)

func main() {
	flag.StringVar(&configFile, "conf-file", "", "path of config file name")
	flag.StringVar(&addr, "addr", "","Ip address of the mock exporter")
	flag.Parse()

	if configFile == "" {
		fmt.Println("Error: Need a config file!")
		os.Exit(1)
	}
	if addr == "" {
		fmt.Println("Error: Need the address of exporter!")
		os.Exit(1)
	}
	f, err := os.Open(configFile)
	if err != nil {
		fmt.Printf("no such file: %s\n", configFile)
		os.Exit(1)
	}
	defer f.Close()

	var config utils.Config
	if bytes, _ := ioutil.ReadAll(f) ; err != nil {
		fmt.Printf("error reading file: %s\n", configFile)
		os.Exit(1)
	} else {
		err = json.Unmarshal(bytes, &config)
		if err != nil {
			fmt.Printf("error parsing json file: %s\n", configFile)
			os.Exit(1)
		}
	}

	utils.Validate(&config)
	bytes, _ := json.Marshal(config)
	data := url.Values{	}
	data.Add("config", string(bytes))
	resp, err := http.PostForm("http://" + addr + ":30576/indicator", data)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
}
