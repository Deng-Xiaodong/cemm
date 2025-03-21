package client

import (
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"bufio"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var emmClient *EMMClient

func init() {
	cf := config.GetDefaultConfig()
	keepAliveArgs := keepalive.ClientParameters{
		Time: 10 * time.Second, // 至少10S，如果10S内没有ping或者数据发送/接收，则触发连接回收
		// 每次ping进行等待的最长时间，keepalive维持一个倒计时器，当触发连接回收并timeout后，连接断开
		Timeout: 20 * time.Second,
	}
	conn, err := grpc.NewClient("127.0.0.1:19090",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepAliveArgs))
	if err != nil {
		log.Fatal(err)
	}
	emmClient = NewEMMClient(0, cf, cemm.NewCEMMClient(conn))
}

func TestEMMClient_Get(t *testing.T) {
	var err error
	_, _, err = emmClient.Get("key1")
	if err != nil {
		log.Fatal(err)
	}
}

func TestEMMClient_Init(t *testing.T) {

	//数据集
	var file *os.File
	var err error
	if file, err = os.Open("multi_map.txt"); err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	rd := bufio.NewScanner(file)
	var length int
	var data [][]string
	for rd.Scan() {
		lineSplit := strings.Split(rd.Text(), " ")
		if len(lineSplit) >= 2 {
			data = append(data, lineSplit)
			length += len(lineSplit) - 1
		}
	}
	err = emmClient.Init(data, length)
	if err != nil {
		log.Fatal(err)
	}
}

func TestConOp(t *testing.T) {
	cf := config.GetDefaultConfig()
	for i := 1; i <= cf.ClientCnt; i++ {
		idx := i
		go conOp(idx, cf)
	}
}

func conOp(c int, cf *config.Config) {
	conn, err := grpc.NewClient("127.0.0.1:19090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	cli := NewEMMClient(c, cf, cemm.NewCEMMClient(conn))

	//模拟并发get
	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, round, err := cli.Get("key" + strconv.Itoa(idx))
			if err != nil {
				log.Println(err)
				return
			}
			log.Printf("round %d got key%d :%v\n", round, idx, res)
			time.Sleep(50 * time.Millisecond)
		}(i)
	}
	for i := 1; i <= 10; i++ {
		for j := 100; j <= 110; j++ {
			err := cli.Add("key"+strconv.Itoa(i), fmt.Sprintf("value%d_%d", i, j*c))
			if err != nil {
				log.Println(err)
				return
			}
		}
	}
	wg.Wait()
}
