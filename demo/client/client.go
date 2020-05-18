package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	conn, _ := net.Dial("tcp", "127.0.0.1:9999")
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	fmt.Println(conn.Write([]byte("12121\r\n")))
	for {
		data := make([]byte, 1024)
		if _, err := conn.Read(data); err != nil {
			fmt.Println(err)
		}
	}

}
