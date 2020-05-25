package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	l, _ := net.Listen("tcp", "127.0.0.1:9999")

	for {
		conn, _ := l.Accept()
		time.Sleep(10 * time.Second)
		fmt.Println(conn.Write([]byte("test 10s\r\n")))
	}
}
