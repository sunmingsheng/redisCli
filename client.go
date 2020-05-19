package redisCli

import (
	"errors"
	"fmt"
	"github.com/howeyc/crc16"
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

const (
	singletonMode = "singleton"
	clusterMode   = "cluster"
)

const (
	SetNx = "NX" //不存在添加
	SetXx = "XX" //存在添加
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
	addr    string
}

//空闲连接对象
type resterConn struct {
	idleTime time.Time //空闲时间点
}

//空闲连接池
type rester struct {
	pool map[string]map[conn]resterConn
	lock sync.Mutex
}

//工作连接池
type worker struct {
	pool map[string]map[conn]struct{}
	lock sync.Mutex
}

type nodeSlot struct {
	start int
	end   int
}

type client struct {
	addr                string              //连接地址
	cluster             []string            //地址
	worker              worker              //工作连接池
	rester              rester              //空闲连接池
	maxOpenConn         int                 //最大连接数
	maxIdleConn         int                 //最大空闲数量
	maxIdleTime         time.Duration       //最长空闲时间
	connectTimeout      time.Duration       //连接超时时间
	readTimeout         time.Duration       //读超时时间
	writeTimeout        time.Duration       //写超时时间
	healthCheckDuration time.Duration       //健康检查周期
	mode                string              //模式
	nodeSlots           map[string]nodeSlot //hash槽映射
	lock                sync.Mutex          //锁
}

var (
	ErrorRespEmpty          = errors.New("响应数据为空")
	ErrorRespTypeNotSupport = errors.New("服务端响应数据格式不支持")
	ErrorAssertion          = errors.New("类型断言失败")
	ErrorKeyNotFound        = errors.New("key不存在")
	ErrorNoConnect          = errors.New("无可用连接")
	ErrorMoved              = errors.New("key slot moved")
	ErrorAsk                = errors.New("key slot ask")
	ErrorSomeWrong          = errors.New("some wrong")
)

//空闲连接channel
var resterChan map[string]chan conn

//key属性
type KeyOption struct {
	LifeTime time.Duration //过期时间
	Mode     string        //添加模式
}

func init() {
	//a := make(map[string]chan conn)
	resterChan = make(map[string]chan conn)  //根据addr的数量创建channel todo
}

//ping命令
func (c *client) Ping() (string, error) {
	cmd := []byte("*1\r\n$4\r\nPING\r\n")
	if resp, err := c.sendCmd("", cmd); err != nil {
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
	cmd := []byte(fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
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

//set命令
func (c *client) Set(key string, val string, option *KeyOption) (bool, error) {
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
	if resp, err := c.sendCmd(key, cmd); err != nil {
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

//del命令
func (c *client) Del(key string) (int, error) {
	cmd := []byte(fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
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
	seconds := strconv.FormatInt(int64(lifeTime.Seconds()), 10)
	cmd := []byte(fmt.Sprintf("*3\r\n$6\r\nEXPIRE\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(seconds), seconds))
	if resp, err := c.sendCmd(key, cmd); err != nil {
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
func (c *client) Exists(key string) (int, error) {
	cmd := []byte(fmt.Sprintf("*%3\r\n$6\r\nEXISTS\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
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
func (c *client) Incr(key string) (int, error) {
	cmd := []byte(fmt.Sprintf("*2\r\n$4\r\nINCR\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
		return 0, err
	} else {
		switch resp.(type) {
		case int:
			return resp.(int), nil
		}
		return 0, ErrorAssertion
	}
}

//decr命令
func (c *client) Decr(key string) (int, error) {
	cmd := []byte(fmt.Sprintf("*2\r\n$4\r\nDECR\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
		return 0, err
	} else {
		switch resp.(type) {
		case int:
			return resp.(int), nil
		}
		return 0, ErrorAssertion
	}
}

//ttl命令
func (c *client) TTL(key string) (int, error) {
	cmd := []byte(fmt.Sprintf("*2\r\n$3\r\nTTL\r\n$%d\r\n%s\r\n", len(key), key))
	if resp, err := c.sendCmd(key, cmd); err != nil {
		return 0, err
	} else {
		switch resp.(type) {
		case int:
			return resp.(int), nil
		}
		return 0, ErrorAssertion
	}
}

//hset命令

//hexists命令

//发送命令
func (c *client) sendCmd(key string, cmd []byte) (interface{}, error) {

	//设置最大尝试次数
	for i := 0; i < 2; i ++ {
		//获取连接
		conn, err := c.getConn(key)
		if err != nil {
			return nil, err
		}

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
			if err == ErrorMoved || err == ErrorAsk {
				if err := c.getClusterNodes(conn); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		} else {
			return v, nil
		}
	}
	return nil, ErrorSomeWrong
}

//关闭工作连接
func (c *client) closeWorkConn(conn conn) {
	c.deleteWorkConn(conn)
	conn.netConn.Close()
}

//删除工作连接
func (c *client) deleteWorkConn(conn conn) {
	c.worker.lock.Lock()
	for addr, conns := range c.worker.pool {
		for key, _ := range conns {
			if key == conn {
				delete(c.worker.pool[addr], key)
				break
			}
		}
	}
	c.worker.lock.Unlock()
}

//获取空闲连接
func (c *client) getRestConn(addr string) (conn, error) {
	c.rester.lock.Lock()
	defer c.rester.lock.Unlock()
	if _, ok := c.rester.pool[addr]; ok && len(c.rester.pool[addr]) > 0 {
		var resterConn conn
		for key := range c.rester.pool[addr] {
			resterConn = key
			break
		}
		if len(c.worker.pool[addr]) < c.maxOpenConn {
			c.worker.lock.Lock()
			c.worker.pool[resterConn.addr][resterConn] = struct{}{}
			c.worker.lock.Unlock()
			c.deleteRestConn(resterConn)
			return resterConn, nil
		} else {
			c.deleteRestConn(resterConn)
			return conn{}, ErrorNoConnect
		}
	}

	//检查是否超出最大连接数
	if len(c.worker.pool[addr]) < c.maxOpenConn {
		if netConn, err := createTcpConn(addr, c.connectTimeout); err != nil {
			return conn{}, err
		} else {
			c.worker.lock.Lock()
			newConn := conn{netConn: netConn, addr: addr}
			if _, ok := c.worker.pool[addr]; !ok {
				c.worker.pool[newConn.addr] = make(map[conn]struct{})
			}
			c.worker.pool[addr][newConn] = struct{}{}
			c.worker.lock.Unlock()
			return newConn, nil
		}
	}
	return conn{}, ErrorNoConnect
}

//添加空闲连接
func (c *client) addRestConn(conn conn) {
	c.rester.lock.Lock()
	c.rester.pool[conn.addr][conn] = resterConn{idleTime: time.Now()}
	c.rester.lock.Unlock()
}

//关闭空闲连接
func (c *client) closeRestConn(conn conn) {
	c.deleteRestConn(conn)
	conn.netConn.Close()
}

//删除空闲连接
func (c *client) deleteRestConn(conn conn) {
	for addr, conns := range c.rester.pool {
		for key, _ := range conns {
			if key == conn {
				delete(c.rester.pool[addr], key)
				break
			}
		}
	}
}

//释放工作连接
func (c *client) releaseWorkConn(conn conn) {
	select {
	case resterChan[conn.addr] <- conn:

	default:
		c.deleteWorkConn(conn)
		c.addRestConn(conn)
	}
}

//心跳检查(资源回收)
func healthCheck(c *client) {
	//todo 定时获取cluster nodes

	cmd := []byte("*1\r\n$4\r\nPING\r\n")
	for {
		time.Sleep(15 * time.Second)
		for _, conns := range c.rester.pool {
			resterConnNum := len(conns)
			for conn, connStatus := range conns {
				if connStatus.idleTime.Add(c.maxIdleTime).Before(time.Now()) && resterConnNum > c.maxIdleConn {
					c.closeRestConn(conn)
				}
				if resp, err := c.sendCmd("", cmd); err != nil {
					resterConnNum -= 1
					c.closeRestConn(conn)
				} else {
					if v, ok := resp.(string); ok && v == "PONG" {
						continue
					} else {
						resterConnNum -= 1
						c.closeRestConn(conn)
					}
				}
			}
		}
	}
}

//发送cluster nodes命令
func (c *client) getClusterNodes(conn conn) error {
	cmd := []byte("*2\r\n$7\r\nCLUSTER\r\n$5\r\nNODES\r\n")
	if resp, err := c.sendCmd("", cmd); err != nil {
		return err
	} else {
		switch resp.(type) {
		case []string:
			nodes := resp.([]string)
			nodeSlots := make(map[string]nodeSlot)
			for _, nodeInfo := range nodes {
				info := strings.Split(nodeInfo, " ")
				if len(info) > 2 && strings.Contains(info[2], "master") {
					if strings.Index(info[1], "@") <= 0 {
						continue
					}
					addr := info[1][:strings.Index(info[1], "@")]
					slots := strings.Split(info[len(info)-1], "-")
					if len(slots) != 2 {
						continue
					}
					start, _ := strconv.Atoi(slots[0])
					end, _ := strconv.Atoi(slots[1])
					nodeSlots[addr] = nodeSlot{
						start: start,
						end:   end,
					}
				}
			}
			c.lock.Lock()
			c.nodeSlots = nodeSlots
			c.lock.Unlock()
			return nil
		}
		return ErrorAssertion
	}
}

//解析redis-serve的响应
func (c *client) parseResp(resp []byte) (interface{}, error) {
	length := len(resp)
	if length <= 1 {
		return nil, ErrorRespEmpty
	}
	switch resp[0] {
	case '-':
		//错误类型
		if string(resp[1:6]) == "MOVED" {
			return nil, ErrorMoved
		}
		if string(resp[1:4]) == "ASK" {
			return nil, ErrorAsk
		}
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
		return strings.Split(string(resp[1:length-2]), "\n"), nil
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

//对数据进行包装
func warpOption(option *Option) {
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
}

//创建客户端
func NewClient(option Option) (*client, error) {

	//处理数据
	warpOption(&option)
	workerPool := make(map[string]map[conn]struct{})
	workerPool[option.Addr] = make(map[conn]struct{})
	resterPool := make(map[string]map[conn]resterConn)
	resterPool[option.Addr] = make(map[conn]resterConn)

	client := &client{
		addr: option.Addr,
		worker: worker{
			pool: workerPool,
			lock: sync.Mutex{},
		},
		rester: rester{
			pool: resterPool,
			lock: sync.Mutex{},
		},
		maxIdleConn:         option.MaxIdleConn,
		maxOpenConn:         option.MaxOpenConn,
		maxIdleTime:         option.MaxIdleTime,
		connectTimeout:      option.ConnectTimeout,
		readTimeout:         option.ReadTimeout,
		writeTimeout:        option.WriteTimeout,
		healthCheckDuration: option.HealthCheckDuration,
		mode:                singletonMode,
	}

	//启动协程，监控所有的连接
	go healthCheck(client)

	return client, nil
}

//创建cluster客户端
func NewClusterClient(option Option) (*client, error) {
	//处理数据
	warpOption(&option)

	workerPool := make(map[string]map[conn]struct{})
	resterPool := make(map[string]map[conn]resterConn)

	for _, addr := range option.Cluster {
		workerPool[addr] = make(map[conn]struct{})
		resterPool[addr] = make(map[conn]resterConn)
		resterChan[addr] = make(chan conn)
	}

	client := &client{
		cluster: option.Cluster,
		worker: worker{
			pool: workerPool,
			lock: sync.Mutex{},
		},
		rester: rester{
			pool: resterPool,
			lock: sync.Mutex{},
		},
		maxIdleConn:         option.MaxIdleConn,
		maxOpenConn:         option.MaxOpenConn,
		maxIdleTime:         option.MaxIdleTime,
		connectTimeout:      option.ConnectTimeout,
		readTimeout:         option.ReadTimeout,
		writeTimeout:        option.WriteTimeout,
		healthCheckDuration: option.HealthCheckDuration,
		mode:                clusterMode,
		nodeSlots:           map[string]nodeSlot{},
		lock:                sync.Mutex{},
	}

	//发送 cluster nodes命令获取集群状态数据
	if conn, err := client.getConn(""); err != nil {
		return nil, err
	} else {
		if err := client.getClusterNodes(conn); err != nil {
			return nil, err
		}
	}
	//启动协程，监控所有的连接
	//go healthCheck(client)

	return client, nil
}

//创建tcp连接
func createTcpConn(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

//获取tcp连接
func (c *client) getConn(key string) (conn, error) {
	addr := ""
	if c.mode == clusterMode {
		if key != "" {
			checksum := crc16.Checksum([]byte(key), crc16.CCITTFalseTable)
			slotNum := int(checksum % 16384)
			for nodeAddr, nodeSlot := range c.nodeSlots {
				if nodeSlot.start <= slotNum && nodeSlot.end >= slotNum {
					addr = nodeAddr
					break
				}
			}
		} else {
			addr = c.cluster[0]
		}
	} else {
		addr = c.addr
	}

	select {
	case conn := <- resterChan[addr]:
		fmt.Println("1212121212")
		return conn, nil
	default:
		//获取新的连接
		return c.getRestConn(addr)
	}
}
