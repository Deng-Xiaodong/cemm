package main

import (
	"DRW/src/rpc/cemm"
	"DRW/src/server"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	// 1. 打开数据库
	opts := badger.DefaultOptions("databases1") // 数据存储在当前目录的 databases 文件夹中
	db, errDb := badger.Open(opts)
	if errDb != nil {
		log.Fatal("Failed to open database: ", errDb)
	}
	var keepAliveArgs = keepalive.ServerParameters{
		Time:             10 * time.Second,
		Timeout:          20 * time.Second,
		MaxConnectionAge: 30 * time.Second,
	}
	listener, errLis := net.Listen("tcp", ":19090")
	if errLis != nil {
		log.Fatalf("failed to listen: %v", errLis)
	}
	s := grpc.NewServer(
		grpc.KeepaliveParams(keepAliveArgs),
		grpc.MaxSendMsgSize(1024*1024*4),
		grpc.MaxRecvMsgSize(1024*1024*4),
	)
	cemm.RegisterCEMMServer(s, server.NewEMMServer(db))

	reflection.Register(s)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Printf("GraceFullyExit has exited, sig:%v\n", sig)
		s.GracefulStop()
	}()
	if err := s.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}
