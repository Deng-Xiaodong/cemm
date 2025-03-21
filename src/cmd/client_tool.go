package main

import (
	"DRW/src/client"
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"log"
	"math/rand"
	"os"
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

	app := &cli.App{
		Name:  "cemm client",
		Usage: "cemm client",
		Commands: []*cli.Command{
			{
				Name:    "add",
				Aliases: []string{"a"},
				Usage:   "添加一个键值对",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "client_id",
						Aliases: []string{"i"},
						Value:   1 + rand.Intn(cf.ClientCnt),
						Usage:   "客户端id，取值为1到n，n为预设的客户端数;不指定则随机",
					},
					&cli.StringFlag{
						Name:     "key",
						Aliases:  []string{"k"},
						Usage:    "待添加的键",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "value",
						Aliases:  []string{"v"},
						Usage:    "待添加的值",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					emmClient := client.NewEMMClient(c.Int("client_id"), cf, cemm.NewCEMMClient(conn))
					err := emmClient.Add(c.String("key"), c.String("value"))
					if err == nil {
						log.Printf("添加[%s, %s]成功!\n", c.String("key"), c.String("value"))
					}
					return err
				},
			},
			{
				Name:    "get",
				Aliases: []string{"g"},
				Usage:   "搜索一个关键字的所有值",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "key",
						Aliases:  []string{"k"},
						Usage:    "待搜索的键",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					emmClient := client.NewEMMClient(0, cf, cemm.NewCEMMClient(conn))
					res, round, err := emmClient.Get(c.String("key"))
					if err == nil {
						log.Printf("round %d got %s :%v\n", round, c.String("key"), res)
					}
					return err
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}
