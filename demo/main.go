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
			"127.0.0.1:7200",
			"127.0.0.1:7201",
			"127.0.0.1:7202",
			"127.0.0.1:7203",
			"127.0.0.1:7204",
			"127.0.0.1:7205",
		},
		MaxIdleTime: time.Second * 10,
		MaxOpenConn: 30,
		MaxIdleConn: 2,
		DataBase: "1",
	}
	client, err := redisCli.NewClusterClient(options)
	if err != nil {
		fmt.Println(err)
	}

	client.AddHook(func(cmd, res []byte, addr string) {
		//fmt.Println(string(cmd))
		//fmt.Println(string(res))
		//fmt.Println(addr)
	})

	go func() {
		for {
			time.Sleep(5 * time.Second)
			//client.Status()
		}
	}()

	http.HandleFunc("/redis", func(writer http.ResponseWriter, request *http.Request) {

		fmt.Println(client.Get("121"))
	})
	http.ListenAndServe(":8080", nil)
}

