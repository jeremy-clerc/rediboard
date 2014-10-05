package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fzzy/radix/redis"
)

/*

/api/instances
/api/instance/:port
/api/hosts -> List of hosts
/api/host/:name -> Return list of instances running on host

curl -s -q http://localhost:8080/api/instances |  python -m json.tool
Response from /api/instances
*/

var HostNames = make(map[string]string)
var Instances = []Instance{}
var LocalConfig = Config{}
var LocalErrors = []string{}

type Cache struct {
	Path       string `json:"path"`
	Expiration string `json:"expiration"`
	Refresh    string `json:"refresh"`
}

type Config struct {
	Cache             Cache      `json:"cache"`
	ConnectionTimeout string     `json:"connection_timeout"`
	Listen            string     `json:"listen"`
	Instances         []Instance `json:"instances"`
}

type Connection struct {
	Port string `json:"port"`
	Host string `json:"host"`
}

//Port            int `json: port`
type Instance struct {
	Port            string       `json:"port"`
	Vip             string       `json:"vip"`
	Errors          []string     `json:"errors"`
	Name            string       `json:"name"`
	Host            string       `json:"host"`
	Auth            string       `json:"auth"`
	Role            string       `json:"role"`
	UsedMemory      int64        `json:"used_memory:"`
	MaxMemory       int64        `json:"maxmemory"`
	MaxMemoryPolicy string       `json:"maxmemory_policy"`
	Version         string       `json:"version"`
	Connections     []Connection `json:"connections"`
	LastUpdated     int64        `json:"last_updated"`
}

func (i *Instance) Address() string {
	return fmt.Sprintf("%s:%s", i.Vip, i.Port)
}

func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res := struct {
		Instances []Instance `json:"instances"`
		Errors    []string   `json:"errors"`
	}{
		Instances,
		LocalErrors,
	}
	answer, err := json.Marshal(res)
	if err != nil {
		log.Fatal("encoder anwser: %v", err)
	}
	w.Write(answer)
}

func getConfigItem(redisCli *redis.Client, item string) (string, bool) {
	reply := redisCli.Cmd("CONFIG", "GET", item)
	if reply.Type == 1 {
		return reply.Err.Error(), false
	}
	values, err := reply.List()
	if err != nil {
		log.Fatal(fmt.Sprintf("CONFIG GET %s [%v]", item, err))
	}
	return values[1], true
}

func errHndlr(err error) {
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func getInstanceInfos(instance *Instance) {
	expiration, err := time.ParseDuration(LocalConfig.Cache.Expiration)
	if err != nil {
		log.Printf("Error while setting expiration time [%v]", err)
		expiration = time.Second * 3600
	}
	if instance.LastUpdated != 0 &&
		time.Since(time.Unix(instance.LastUpdated, 0)) < expiration {
		return
	}
	instance.LastUpdated = time.Now().Unix()
	// We reinitialize the Connections array, or we get duplicate
	instance.Connections = []Connection{}
	// We reinitialize the error array
	instance.Errors = []string{}
	timeout, err := time.ParseDuration(LocalConfig.ConnectionTimeout)
	if err != nil {
		log.Printf("Error while setting Dial timeout [%v]", err)
		timeout = time.Millisecond * 500
	}
	redisCli, err := redis.DialTimeout("tcp", instance.Address(), timeout)
	if err != nil {
		errStr := fmt.Sprintf("Connection error [%v]", err)
		log.Println(errStr)
		instance.Errors = append(instance.Errors, errStr)
		return
	}
	defer redisCli.Close()
	if len(instance.Auth) > 0 {
		reply := redisCli.Cmd("AUTH", instance.Auth)
		if reply.Type == 1 {
			log.Printf("Authentication error for %s:%s [%v]", instance.Vip, instance.Port, reply.Err.Error())
			instance.Errors = append(instance.Errors, reply.Err.Error())
			return
		}
	}

	instance.Host = getHostname(instance.Vip)
	if replyContent, ok := getConfigItem(redisCli, "maxmemory"); ok {
		instance.MaxMemory, err = strconv.ParseInt(replyContent, 0, 64)
		if err != nil {
			log.Fatal("Failed conversion of maxmemory str to int [%v]", err)
		}
	} else {
		log.Printf("CONFIG GET maxmemory error for %s:%s [%s]", instance.Vip, instance.Port, replyContent)
		instance.Errors = append(instance.Errors, replyContent)
		return
	}
	if replyContent, ok := getConfigItem(redisCli, "maxmemory-policy"); ok {
		instance.MaxMemoryPolicy = replyContent
	} else {
		log.Printf("CONFIG GET maxmemory-policy error for %s:%s [%s]", instance.Vip, instance.Port, replyContent)
		instance.Errors = append(instance.Errors, replyContent)
		return
	}

	reply := redisCli.Cmd("INFO")
	if reply.Type == 1 {
		log.Printf("INFO error for  %s:%s [%v]", instance.Vip, instance.Port, reply.Err.Error())
		instance.Errors = append(instance.Errors, reply.Err.Error())
		return
	}
	replycontent, err := reply.Str()
	errHndlr(err)
	lines := strings.Split(replycontent, "\r\n")
	for index, line := range lines {
		if len(line) == 0 || string(line[0]) == "#" {
			continue
		}
		element := strings.Split(line, ":")
		if element[0] == "redis_version" {
			instance.Version = element[1]
		} else if element[0] == "used_memory" {
			instance.UsedMemory, err = strconv.ParseInt(element[1], 0, 64)
			errHndlr(err)
		} else if element[0] == "role" {
			instance.Role = element[1]
			if instance.Role == "slave" {
				masterhost := strings.Split(lines[index+1], ":")
				masterport := strings.Split(lines[index+2], ":")
				instance.Connections = append(instance.Connections,
					Connection{Host: getHostname(masterhost[1]),
						Port: masterport[1]})
			}
		} else {
			matched, err := regexp.MatchString("^slave[0-9]+", element[0])
			errHndlr(err)
			if matched {
				if strings.Contains(instance.Version, "2.6") {
					extract := strings.Split(element[1], ",")
					instance.Connections = append(instance.Connections,
						Connection{Host: getHostname(extract[0]),
							Port: extract[1]})
				} else {
					re := regexp.MustCompile("ip=([0-9.]+),port=([0-9]+),.*")
					extract := re.FindStringSubmatch(element[1])
					instance.Connections = append(instance.Connections,
						Connection{Host: getHostname(extract[1]),
							Port: extract[2]})
				}
			}
		}
	}
}

func getHostname(host string) string {
	if net.ParseIP(host) == nil {
		addrs, err := net.LookupHost(host)
		errHndlr(err)
		host = addrs[0]
	}
	if name, ok := HostNames[host]; ok {
		return name
	} else {
		names, err := net.LookupAddr(host)
		errHndlr(err)
		HostNames[host] = names[0]
		return names[0]
	}
}

func isCached(port string, cachedInstances []Instance) (Instance, bool) {
	for _, instance := range cachedInstances {
		if port == instance.Port {
			return instance, true
		}
	}
	return Instance{}, false
}

// Creating a temporary refreshedInstances because I do not seem to be able
// to update the main var Instances
func RefreshInstances() {
	refresh, err := time.ParseDuration(LocalConfig.Cache.Refresh)
	if err != nil {
		log.Printf("Error while setting refresh time [%v]", err)
		refresh = time.Second * 300
	}
	for {
		time.Sleep(refresh)
		refreshedInstances := []Instance{}
		for _, instance := range Instances {
			getInstanceInfos(&instance)
			refreshedInstances = append(refreshedInstances, instance)
		}
		Instances = refreshedInstances
	}
}

func Init() {
	//Instances
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s /some/path/to/config.json", os.Args[0])
		os.Exit(1)
	}
	ReadConfig(os.Args[1])
	var cachedInstances = ReadCache()
	for _, instance := range LocalConfig.Instances {
		if cachedInstance, ok := isCached(instance.Port, cachedInstances); ok {
			instance = cachedInstance
		}
		getInstanceInfos(&instance)
		Instances = append(Instances, instance)
	}
	// Here is an automic refresh every five minutes
	go RefreshInstances()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			log.Printf("Received %s, saving cache...", sig)
			WriteCache()
			os.Exit(0)
		}
	}()
}

func ReadConfig(path string) {
	source, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal("Error while reading configuration file [%s]: %v", path, err)
	}
	err = json.Unmarshal(source, &LocalConfig)
	if err != nil {
		log.Fatal("Error while decoding configuration file: %v", err)
	}
}

func ReadCache() []Instance {
	var instances = []Instance{}
	source, err := ioutil.ReadFile(LocalConfig.Cache.Path)
	if os.IsNotExist(err) {
		return instances
	}
	if err != nil {
		log.Fatal("Error while reading cache file: %v", err)
	}
	err = json.Unmarshal(source, &instances)
	if err != nil {
		log.Fatal("Error while decoding cache file: %v", err)
	}
	return instances
}

func WriteCache() {
	content, err := json.Marshal(Instances)
	if err != nil {
		log.Fatal("Encoding cache content: %v", err)
	}
	err = ioutil.WriteFile(LocalConfig.Cache.Path, content, 0644)
	if err != nil {
		log.Fatal("Error while writing cache file: %v", err)
	}
}

func main() {
	Init()
	log.Printf("Initialization done, starting to listen on %s", LocalConfig.Listen)
	http.HandleFunc("/api/instances", getInstancesHandler)
	http.ListenAndServe(LocalConfig.Listen, nil)
}
