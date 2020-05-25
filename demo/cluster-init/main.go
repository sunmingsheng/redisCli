package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

func main() {

	port := 7200
	confPath := "/usr/local/var/go/redisCli/demo/cluster-init/redis.conf"
	file, err := os.Open(confPath)
	if err != nil {
		panic(err)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}

	cluster := "redis-cli --cluster create --cluster-replicas 1 "
	for i := 0; i < 6; i++ {
		portS := strconv.Itoa(port + i)
		s := strings.ReplaceAll(string(content), "cluster-config-file nodes.conf", "cluster-config-file nodes-" + portS + ".conf")
		s = strings.ReplaceAll(s, "port 6379", "port " + portS)
		filePath := "/usr/local/var/go/redisCli/demo/cluster-init/redis-" + portS + ".conf"
		file, err := os.Create(filePath)
		if err != nil {
			panic(err)
		}
		if _, err := file.Write([]byte(s)); err != nil {
			panic(err)
		}
		cluster += "127.0.0.1:" + portS + " "
		fmt.Println("nohup redis-server " + filePath + " &")
	}

	fmt.Println(cluster)
}
