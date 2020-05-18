package redisCli

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxOpenConn         = 3
	defaultMaxIdleConn         = 1
	defaultMaxIdleTime         = 5 * time.Minute
	defaultConnectTimeout      = 3 * time.Second
	defaultReadTimeout         = 2 * time.Second
	defaultWriteTimeout        = 2 * time.Second
	defaultHealthCheckDuration = 15 * time.Second
)

var (
	ErrorRespEmpty          = errors.New("响应数据为空")
	ErrorRespTypeNotSupport = errors.New("服务端响应数据格式不支持")
	ErrorAssertion          = errors.New("类型断言失败")
	ErrorKeyNotFound        = errors.New("key不存在")
	ErrorNoConnect          = errors.New("无可用连接")
)

//redis 连接池的实现
type Option struct {
	Addr                string        //连接地址
	Cluster             []string      //cluster集群配置
	MaxOpenConn         int           //最大连接数
	MaxIdleConn         int           //最大空闲数量
	MaxIdleTime         time.Duration //最长空闲时间
	ConnectTimeout      time.Duration //连接超时时间
	ReadTimeout         time.Duration //tcp读超时时间
	WriteTimeout        time.Duration //tcp写超时时间
	HealthCheckDuration time.Duration //健康检查周期
}

type conn struct {
	netConn net.Conn
}

//空闲连接对象
type restConn struct {
	idleTime time.Time //空闲时间点
}

//空闲连接池
type restPool struct {
	pool map[conn]restConn
	lock sync.Mutex
}

//工作连接池
type workPool struct {
	pool map[conn]struct{}
	lock sync.Mutex
}

type client struct {
	addr                string        //连接地址
	workPool            workPool      //工作连接池
	restPool            restPool      //空闲连接池
	maxOpenConn         int           //最大连接数
	maxIdleConn         int           //最大空闲数量
	maxIdleTime         time.Duration //最长空闲时间
	connectTimeout      time.Duration //连接超时时间
	readTimeout         time.Duration //读超时时间
	writeTimeout        time.Duration //写超时时间
	healthCheckDuration time.Duration //健康检查周期
}

const (
	SetNx = "NX" //不存在添加
	SetXx = "XX" //存在添加
)

//空闲连接channel
var restChan chan conn

//key属性
type KeyOption struct {
	LifeTime time.Duration //过期时间
	Mode     string        //添加模式
}

func init() {
	restChan = make(chan conn)
}

//ping命令
func (c *client) Ping() (string, error) {
	conn, err := c.getConn()
	if err != nil {
		return "", err
	}
	cmd := []byte("*1\r\n$4\r\nPING\r\n")
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return "", err
	} else {
		if v, ok := resp.(string); ok {
			return v, nil
		} else {
			return "", ErrorAssertion
		}
	}
}

//get命令
func (c *client) Get(key string) (string, error) {
	conn, err := c.getConn()
	if err != nil {
		return "", err
	}
	cmd := []byte(fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return "", err
	} else {
		switch resp.(type) {
		case string:
			v := resp.(string)
			if v == "-1" {
				return "", ErrorKeyNotFound
			}
			return v, nil
		case []string:
			v := resp.([]string)
			return v[1], nil
		}
		return "", ErrorAssertion
	}
}

//mget命令
func (c *client) Mget(keys ...string) ([]interface{}, error) {
	conn, err := c.getConn()
	if err != nil {
		return nil, err
	}
	cmd := []byte(fmt.Sprintf("*%d\r\n$4\r\nMGET\r\n", len(keys)+1))
	for _, value := range keys {
		cmd = append(cmd, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value))...)
	}
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return nil, err
	} else {
		res := []interface{}{}
		switch resp.(type) {
		case []string:
			for _, value := range resp.([]string) {
				if value == "-1" {
					res = append(res, false)
				} else {
					res = append(res, value)
				}
			}
			return res, nil
		}
		return nil, ErrorAssertion
	}
}

//set命令
func (c *client) Set(key string, val string, option *KeyOption) (bool, error) {
	conn, err := c.getConn()
	if err != nil {
		return false, err
	}
	cmd := []byte(fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val))
	if option != nil {
		varNum := 3
		if option.LifeTime != 0 {
			seconds := strconv.FormatInt(int64(option.LifeTime.Seconds()), 10)
			cmd = append(cmd, []byte(fmt.Sprintf("$2\r\nEX\r\n$%d\r\n%s\r\n", len(seconds), seconds))...)
			varNum += 2
		}
		switch option.Mode {
		case SetNx:
			cmd = append(cmd, []byte("$2\r\nNX\r\n")...)
			varNum += 1
		case SetXx:
			cmd = append(cmd, []byte("$2\r\nXX\r\n")...)
			varNum += 1
		}
		cmd = append([]byte(fmt.Sprintf("*%d", varNum)), cmd[2:]...)
	}
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return false, err
	} else {
		switch resp.(type) {
		case string:
			v := resp.(string)
			if v == "OK" {
				return true, nil
			}
			return false, nil
		}
		return false, ErrorAssertion
	}
}

//mset命令
func (c *client) Mset(kvs map[string]string) (bool, error) {
	conn, err := c.getConn()
	if err != nil {
		return false, err
	}
	cmd := []byte(fmt.Sprintf("*%d\r\n$4\r\nMSET\r\n", len(kvs)*2+1))
	for key, value := range kvs {
		cmd = append(cmd, []byte(fmt.Sprintf("$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(value), value))...)
	}
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return false, err
	} else {
		switch resp.(type) {
		case string:
			if resp.(string) == "OK" {
				return true, nil
			}
			return false, nil
		}
		return false, ErrorAssertion
	}
}

//del命令
func (c *client) Del(keys ...string) (int, error) {
	conn, err := c.getConn()
	if err != nil {
		return 0, err
	}
	cmd := []byte(fmt.Sprintf("*%d\r\n$3\r\nDEL\r\n", len(keys)+1))
	for _, value := range keys {
		cmd = append(cmd, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value))...)
	}
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return 0, err
	} else {
		switch resp.(type) {
		case int:
			return resp.(int), nil
		}
		return 0, ErrorAssertion
	}
}

//expire命令
func (c *client) Expire(key string, lifeTime time.Duration) (bool, error) {
	conn, err := c.getConn()
	if err != nil {
		return false, err
	}
	seconds := strconv.FormatInt(int64(lifeTime.Seconds()), 10)
	cmd := []byte(fmt.Sprintf("*3\r\n$6\r\nEXPIRE\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(seconds), seconds))
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return false, err
	} else {
		switch resp.(type) {
		case int:
			if resp.(int) == 1 {
				return true, nil
			}
			return false, nil
		}
		return false, ErrorAssertion
	}
}

//exists命令
func (c *client) Exists(keys ...string) (int, error) {
	conn, err := c.getConn()
	if err != nil {
		return 0, err
	}
	cmd := []byte(fmt.Sprintf("*%d\r\n$6\r\nEXISTS\r\n", len(keys)+1))
	for _, value := range keys {
		cmd = append(cmd, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value))...)
	}
	if resp, err := c.sendCmd(conn, cmd); err != nil {
		return 0, err
	} else {
		switch resp.(type) {
		case int:
			return resp.(int), nil
		}
		return 0, ErrorAssertion
	}
}

//incr命令
//func (c *client) Incr(key string) (int, error) {
//
//}
//
//
////decr命令
//func (c *client) Decr(key string) (int, error) {
//
//}
//
////ttl命令
//func (c *client) Ttl(key string) (int, error) {
//
//}
//
////type命令
//func (c *client) Type(key string) (int, error) {
//
//}

//发送命令
func (c *client) sendCmd(conn conn, cmd []byte) (interface{}, error) {
	//设置tcp读超时
	if err := conn.netConn.SetReadDeadline(time.Now().Add(c.readTimeout)); err != nil {
		go c.closeWorkConn(conn)
		return nil, err
	}
	//设置tcp写超时
	if err := conn.netConn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
		go c.closeWorkConn(conn)
		return nil, err
	}
	//写入数据
	if _, err := conn.netConn.Write(cmd); err != nil {
		go c.closeWorkConn(conn)
		return nil, err
	}
	//读取数据
	data := []byte{}
	for {
		size := 1024
		temp := make([]byte, size)
		length, err := conn.netConn.Read(temp)
		if err != nil {
			go c.closeWorkConn(conn)
			return nil, err
		}
		if length < size {
			data = append(data, temp[0:length]...)
			if string(data[len(data)-2:]) == "\r\n" {
				break
			}
		} else {
			data = append(data, temp...)
		}
	}
	//释放工作连接
	go c.releaseWorkConn(conn)

	//解析数据
	if v, err := c.parseResp(data); err != nil {
		return nil, err
	} else {
		return v, nil
	}
}

//关闭工作连接
func (c *client) closeWorkConn(conn conn) {
	c.workPool.lock.Lock()
	conn.netConn.Close()
	delete(c.workPool.pool, conn)
	c.workPool.lock.Unlock()
}

//关闭空闲连接
func (c *client) closeRestConn(conn conn) {
	c.restPool.lock.Lock()
	conn.netConn.Close()
	delete(c.restPool.pool, conn)
	c.restPool.lock.Unlock()
}

//释放工作连接
func (c *client) releaseWorkConn(conn conn) {
	select {
	case restChan <- conn:

	case <-time.After(5 * time.Second):
		c.workPool.lock.Lock()
		delete(c.workPool.pool, conn)
		c.workPool.lock.Unlock()
		c.restPool.lock.Lock()
		c.restPool.pool[conn] = restConn{idleTime: time.Now()}
		c.restPool.lock.Unlock()
	}
}

//心跳检查(资源回收)
func healthCheck(c *client) {
	cmd := []byte("*1\r\n$4\r\nPING\r\n")
	for {
		time.Sleep(15 * time.Second)
		restConnNum := len(c.restPool.pool)
		for conn, status := range c.restPool.pool {
			if status.idleTime.Add(c.maxIdleTime).Before(time.Now()) && restConnNum > c.maxIdleConn {
				c.closeRestConn(conn)
				restConnNum -= 1
				continue
			}
			if resp, err := c.sendCmd(conn, cmd); err != nil {
				restConnNum -= 1
				c.closeRestConn(conn)
			} else {
				if v, ok := resp.(string); ok && v == "PONG" {
					continue
				} else {
					restConnNum -= 1
					c.closeRestConn(conn)
				}
			}
		}
	}
}

//解析redis-serve的响应
func (c *client) parseResp(resp []byte) (interface{}, error) {
	fmt.Println(string(resp))
	length := len(resp)
	if length <= 1 {
		return nil, ErrorRespEmpty
	}
	switch resp[0] {
	case '-':
		//错误类型
		return nil, errors.New(string(resp[1 : length-2]))
	case ':':
		//整数回复
		if num, err := strconv.Atoi(string(resp[1 : length-2])); err != nil {
			return nil, err
		} else {
			return num, nil
		}
	case '+':
		//响应为普通字符串
		return string(resp[1 : length-2]), nil
	case '$':
		//批量回复
		if resp[1] == '-' {
			return string(resp[1 : length-2]), nil
		}
		return strings.Split(string(resp[1:length-2]), "\r\n"), nil
	case '*':
		//数组
		s := strings.Split(string(resp), "\r\n")
		res := []string{}
		for i := 1; i < len(s); i++ {
			if i%2 == 1 && len(s[i]) > 0 && s[i][0] == '$' {
				if (s[i][1:]) == "-1" {
					res = append(res, "-1")
				} else {
					res = append(res, s[i+1])
				}
			}
		}
		return res, nil
	}
	return nil, ErrorRespTypeNotSupport
}

//创建客户端
func NewClient(option Option) (*client, error) {
	if option.MaxIdleConn == 0 {
		option.MaxIdleConn = defaultMaxIdleConn
	}
	if option.MaxOpenConn == 0 {
		option.MaxOpenConn = defaultMaxOpenConn
	}
	if option.MaxIdleTime == 0 {
		option.MaxIdleTime = defaultMaxIdleTime
	}
	if option.ConnectTimeout == 0 {
		option.ConnectTimeout = defaultConnectTimeout
	}
	if option.ReadTimeout == 0 {
		option.ReadTimeout = defaultReadTimeout
	}
	if option.WriteTimeout == 0 {
		option.WriteTimeout = defaultWriteTimeout
	}
	if option.HealthCheckDuration == 0 {
		option.HealthCheckDuration = defaultHealthCheckDuration
	}

	client := &client{
		addr: option.Addr,
		workPool: workPool{
			pool: map[conn]struct{}{},
			lock: sync.Mutex{},
		},
		restPool: restPool{
			pool: map[conn]restConn{},
			lock: sync.Mutex{},
		},
		maxIdleConn:         option.MaxIdleConn,
		maxOpenConn:         option.MaxOpenConn,
		maxIdleTime:         option.MaxIdleTime,
		connectTimeout:      option.ConnectTimeout,
		readTimeout:         option.ReadTimeout,
		writeTimeout:        option.WriteTimeout,
		healthCheckDuration: option.HealthCheckDuration,
	}

	//启动协程，监控所有的连接
	go healthCheck(client)

	return client, nil
}

//创建tcp连接
func createTcpConn(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

//获取tcp连接
func (c *client) getConn() (conn, error) {
	select {
	case conn := <-restChan:
		c.workPool.lock.Lock()
		delete(c.workPool.pool, conn)
		c.workPool.lock.Unlock()
		return conn, nil
	default:
		c.restPool.lock.Lock()
		defer c.restPool.lock.Unlock()
		c.workPool.lock.Lock()
		defer c.workPool.lock.Unlock()
		if len(c.restPool.pool) > 0 {
			var restConn conn
			for key := range c.restPool.pool {
				restConn = key
				break
			}
			if len(c.workPool.pool) < c.maxOpenConn {
				c.workPool.pool[restConn] = struct{}{}
				delete(c.restPool.pool, restConn)
				return restConn, nil
			} else {
				delete(c.restPool.pool, restConn)
				return conn{}, ErrorNoConnect
			}
		}
		//检查是否超出最大连接数
		if len(c.workPool.pool) < c.maxOpenConn {
			if netConn, err := createTcpConn(c.addr, c.connectTimeout); err != nil {
				return conn{}, err
			} else {
				conn := conn{netConn: netConn}
				c.workPool.pool[conn] = struct{}{}
				return conn, nil
			}
		}
	}
	return conn{}, ErrorNoConnect
}

func (c *client) Status() {
	fmt.Println(fmt.Sprintf("空闲连接数量:%d,工作连接数量:%d", len(c.restPool.pool), len(c.workPool.pool)))
}
