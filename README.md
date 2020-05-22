# redisCli

redisCli是 golang redis 客户端，支持单点redis以及cluster模式，支持自定义Hook，支持设置连接池属性(最大连接数，最小空闲连接数，连接最大空闲时间等)，支持心跳检查等，使用起来也比较简单。



AddHook() //添加自定义Hook 

Status() //查看当前实例状态，包括ip以及空闲，工作连接池等

Ping() //对应redis的 ping 命令

                             
 
 
 
