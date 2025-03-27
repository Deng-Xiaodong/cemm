package main

import (
	"DRW/src/client"
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"bufio"
	"fmt"
	"github.com/urfave/cli/v2"
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
	var emmClient *client.EMMClient

	app := &cli.App{
		Name:  "cemm-client",
		Usage: "cemm交互式客户端工具，需要指定客户端ID，支持添加add和查询get",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "初始化一个客户端",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:     "id",
						Usage:    "客户端ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					emmClient = client.NewEMMClient(c.Int("id"), cf, cemm.NewCEMMClient(conn))
					startInteractiveCLI(emmClient)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}

// 3. 交互式 CLI 逻辑
func startInteractiveCLI(emmClient *client.EMMClient) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("欢迎使用 CEMMClientCLI！输入 'quit' 或 'exit' 退出。")

	for {
		fmt.Print("cemm> ")
		if !scanner.Scan() { // 读取输入
			break // 用户按下 Ctrl+D 或发生错误
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// 处理退出命令
		if input == "quit" || input == "exit" {
			fmt.Println("再见！")
			break
		}

		// 4. 解析并执行命令
		args := strings.Fields(input)
		cmd := args[0]
		switch cmd {
		case "add":
			if len(args) < 3 {
				fmt.Println("错误: 用法 -> add key value...")
				continue
			}
			key, values := args[1], args[2:]
			for _, v := range values {
				if err := emmClient.Add(key, v); err != nil {
					fmt.Printf("添加失败，错误原因: %v\n", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			fmt.Println("OK")

		case "get":
			if len(args) != 2 {
				fmt.Println("错误: 用法 -> get key")
				continue
			}
			key := args[1]
			if value, err := emmClient.Get(key); err == nil {
				fmt.Printf("%v\n", value)
			} else {
				fmt.Printf("查询失败，错误原因: %v\n", err)
			}
		default:
			fmt.Printf("错误: 未知命令 '%s'\n", cmd)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("输入错误:", err)
	}
}
