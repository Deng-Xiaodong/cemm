package main

import (
	"DRW/src/client"
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"log"
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
	emmClient.Get("key1")
}
