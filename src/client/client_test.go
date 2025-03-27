package client

import (
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"strconv"
	"sync"
	"testing"
)

func TestConOp(t *testing.T) {
	cf := config.GetDefaultConfig()
	var wg sync.WaitGroup
	for i := 1; i <= cf.ClientCnt; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conOp(idx, cf)
		}(i)
	}
	wg.Wait()
}

func conOp(c int, cf *config.Config) {
	conn, err := grpc.NewClient("127.0.0.1:19090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	cli := NewEMMClient(c, cf, cemm.NewCEMMClient(conn))

	if c == 1 {
		for i := 1; i <= 1000; i++ {
			err := cli.Add("key1", "value1_"+strconv.Itoa(i+2000))
			if err != nil {
				log.Println(err)
				return
			}
			//time.Sleep(10 * time.Millisecond)
		}

	} else if c == 2 {
		for i := 1; i <= 1000; i++ {
			err := cli.Add("key1", "value1_"+strconv.Itoa(i+3000))
			if err != nil {
				log.Println(err)
				return
			}
			//time.Sleep(10 * time.Millisecond)
		}

	} else {
		for i := 1; i <= 100; i++ {
			//delay := 10 + rand.Intn(20)
			//time.Sleep(time.Duration(delay) * time.Millisecond)
			res, _ := cli.Get("key1")
			//sort.Slice(res, func(i, j int) bool {
			//	a := strings.Split(res[i], "_")[1]
			//	b := strings.Split(res[j], "_")[1]
			//	return compareStringIntsn(a, b)
			//})
			log.Printf("cli%d search key1 got :%v\n", c, res)
		}
	}

}

func compareStringIntsn(a, b string) bool {
	// 1. 比较长度
	if len(a) < len(b) {
		return false
	} else if len(a) > len(b) {
		return true
	}

	// 2. 长度相同，逐字符比较
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return false
		} else if a[i] > b[i] {
			return true
		}
	}
	return true // 完全相等
}
