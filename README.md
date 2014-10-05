rediboard
=========

Redis Dashboard - Know where your instances run
### Technologies
* Go (http://golang.org/) for the backend API
* Angular.js (https://angularjs.org/) and bootstrap (http://getbootstrap.com/) for the frontend

An `nginx.example` vhost file can be found in the examples folder to serve your frontend and serve the `/api` request to your backend. This can be easily coverted to Apache.

### What you can see and do

You can see a list of instances with some info.

For each instance you can see on which host it is running, on which host its slaves are running and on which port they listen. You also get the maxmemory and maxmemory-policy (eviction policy)

You can also search your instances by any field: Vip, port, host, memory policy, ...

You can sort your instances by Name, port and host.

![Screenshot](https://raw.github.com/jeremy-clerc/rediboard/master/screenshots/rediboard.png)

### Cache and autorefresh

Instances info are fetched when starting the API, then everything is cached in memory for the time you put in the configuration (`expire`). A go routine check every N seconds/minutes if any info is expired and so refresh it (`refresh`)

### Configuration

Configuration is in JSON for two reasons:

1. JSON is part of Go core (http://golang.org/pkg/encoding/json/)
2. We serve JSON for the frontend, less work to implement another configuration format.

You can find an example in `examples/config.json.example`, but here are some explanation about the different part:

#### Time format

The format to use for `connection_timeout`, `expiration`, `refresh` is specified in http://golang.org/pkg/time/#ParseDuration

#### Global

* `"connection_timeout": "500ms"` Redis connection timeout
* `"listen": "127.0.0.1:8080"` Format IP:Port, will listen on the ip and port

#### Cache (`"cache: {}"`)

*  `"path": "./cache.json"` When you stop the API via `SIGINT` (or `CTRL-C` if running in foreground), it saves all the cached instances info in this file, so if you restart for any reason and cache has not expired, you do not need to get again all the info about your 100 instances.
* `"expiration": "1h"` For how much time you estimate the instances info are valid.
* `"refresh": "5m"` Rate at which it is checked that the instances info are expired and so we need to refresh them.

#### Instances (`"instances: []"`)

* `"vip": 192.168.1.23` IP or FQDN to reach the instance
* `"port": "6379"` Which the instance listens on (string format)
* `"name": "The big cache"` Name to show with the `vip:port` on the dashboard
* `"auth": "password"` Specify what you put in front of `requirepass` in your redis config

```json
  "instances": [{
     "vip": "my-redis-instance.example.com",
     "port": "6379", 
     "name": "My cache"
  },{
     "vip": "my-redis-instance-02.example.com",
     "port": "6382", 
     "name": "Second project",
     "auth": "bla"
  }]
```

### Run

Configure your frontend server/proxy like specified in the `nginx.example` to serve the frontend directory and to forward the `/api` request to your backend API started via:

```
go get github.com/jeremy-clerc/rediboard
$GOPATH/bin/rediboard configuration.json
```

