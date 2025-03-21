package main

import (
	"DRW/src/client"
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"bufio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"log"
	"os"
	"strings"
	"time"
)

func main() {

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
	defer conn.Close()
	cf := config.GetDefaultConfig()
	emmClient := client.NewEMMClient(0, cf, cemm.NewCEMMClient(conn))
	//数据集
	var file *os.File
	if file, err = os.Open("data/multi_map.txt"); err != nil {
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
