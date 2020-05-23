# redisCli

redisCli是一个 golang redis 客户端，支持单点redis以及cluster模式，支持自定义Hook，支持设置连接池属性(最大连接数，最小空闲连接数，连接最大空闲时间等)，支持心跳检查等，使用起来也比较简单。

1:创建一个redis客户端
   
   (1):单机模式
   ```golang
    options := redisCli.Option{
		    Addr:"127.0.0.1:6379",
		    MaxIdleTime: time.Second * 1000,
		    MaxOpenConn: 30,
		    MaxIdleConn: 2,
	}
   client, err := redisCli.NewClient(options)
   ``` 
   (2):cluster模式
   ```golang
   options := redisCli.Option{
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
	}
   client, err := redisCli.NewClient(options)
   ```

2:添加自定义Hook

```golang
client.AddHook(func(cmd, res []byte, addr string) {
	fmt.Println(string(cmd))  //打印命令
	fmt.Println(string(res))  //打印响应
	fmt.Println(addr)         //打印地址
})
```

3:查询当前实例状态，包括ip以及空闲，工作连接池等
```golang
go func() {
	for {
		time.Sleep(5 * time.Second)
		client.Status()
	}
}()
```

4:其他方法的使用
```golang
fmt.Println(client.Get("age"))
fmt.Println(client.HLen("12"))
```
                           
5:目前支持的redis命令
+ ping
+ set
+ get
+ del
+ expire
+ ttl
+ exists
+ incr
+ decr
+ hset
+ hget
+ hdel
+ hlen
+ hexists
 
 
