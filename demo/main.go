package main

import (
	"fmt"
	"net/http"
	"redisCli"
	"time"
)

func main() {

	options := redisCli.Option{
		Addr:"127.0.0.1:6379",
		Cluster:[]string{
			"127.0.0.1:7100",
			"127.0.0.1:7101",
			"127.0.0.1:7102",
			"127.0.0.1:7103",
			"127.0.0.1:7104",
			"127.0.0.1:7105",
		},
		MaxIdleTime: time.Second * 10,
		MaxOpenConn: 30,
		MaxIdleConn: 2,
	}
	client, _ := redisCli.NewClusterClient(options)

	go func() {
		for {
			time.Sleep(5 * time.Second)
			client.Status()
		}
	}()

	num := 0
	http.HandleFunc("/redis", func(writer http.ResponseWriter, request *http.Request) {

		if _, err := client.Get("age"); err != nil && err.Error() == "无可用连接" {
			num +=1
		}
		if _, err := client.Get("222"); err != nil && err.Error() == "无可用连接" {
			num +=1
		}
		fmt.Println(num)
	})
	http.ListenAndServe(":8080", nil)
}

