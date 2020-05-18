package main

import (
	"context"
	"fmt"
	"net/http"
	"redisCli"
	"time"
)

func main() {

	options := redisCli.Option{
		Addr:"127.0.0.1:6379",
		MaxIdleTime: time.Second * 10,
		MaxOpenConn: 50,
		MaxIdleConn: 2,
	}
	client, _ := redisCli.NewClient(options)

	//data := []byte("age")
	//checksum := crc16.Checksum(data, crc16.SCSITable)
	//fmt.Println(checksum)
	//fmt.Println(checksum % 16384)

	go func() {
		for {
			time.Sleep(5 * time.Second)
			client.Status()
		}
	}()

	//CRC16(key) %16384
	http.HandleFunc("/redis", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Println(client.Set("6666", "1212", &redisCli.KeyOption{
			LifeTime: 1000 * time.Second,
			Mode:     redisCli.SetNx,
		}))
	})
	http.ListenAndServe(":8080", nil)
}

func test(ctx context.Context) {
	time.Sleep(9 * time.Second)
	fmt.Println(444)
}
