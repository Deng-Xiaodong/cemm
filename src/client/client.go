package client

import (
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"DRW/src/util"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"
	"time"
)

type EMMClient struct {
	cnt, c, volume, limit int
	state                 map[string]int
	//方案暂时假定后续添加不能超过初始化的关键字空间
	//doneInit              map[string]bool
	stub cemm.CEMMClient
}
type GetToken struct {
	LastAddr []byte
}

func NewEMMClient(c int, cf *config.Config, stub cemm.CEMMClient) *EMMClient {
	return &EMMClient{
		cnt:    cf.ClientCnt,
		c:      c,
		volume: cf.Volume,
		limit:  cf.Limit,
		state:  make(map[string]int),
		stub:   stub,
	}
}

func (c *EMMClient) getClientRoundStart(round int) int {
	return c.cnt*c.volume*(round-1) + c.volume*(c.c-1) + 1
}
func (c *EMMClient) getClientRoundEnd(round int) int {
	return c.cnt*c.volume*(round-1) + c.volume*c.c
}
func (c *EMMClient) getClientRoundEndWithId(round, id int) int {
	return c.cnt*c.volume*(round-1) + c.volume*id
}
func (c *EMMClient) getAllClientRoundEnd(round int) (sts []int) {
	for i := 1; i <= c.cnt; i++ {
		sts = append(sts, c.getClientRoundEndWithId(round, i))
	}
	return
}
func (c *EMMClient) getRoundEnd(round int) int {
	return c.cnt * c.volume * round
}

func (c *EMMClient) genAddr(stKey []byte, i int) []byte {
	st := fmt.Sprintf("%x%d", stKey, i)
	return util.H1(string(util.H2(st)))
}
func (c *EMMClient) genKeywordMask(keyword string) []byte {
	return util.H1(keyword)
}
func (c *EMMClient) genAddToken(keyword, value string, round int, mask []byte) []*cemm.AddToken {

	cipherValue, err := util.AESEncryptCBC(mask[:16], []byte(value))
	if err != nil {
		log.Fatalln(err)
	}
	//获取关键字计数
	//if c.state[keyword] == nil {
	//	c.state[keyword] = make(map[int]int)
	//}
	//st = c.state[keyword][round]
	//if st == 0 {
	//	st = c.getClientRoundStart(round)
	//	c.state[keyword][round] = st
	//}
	//c.state[keyword][round]++

	st := c.state[keyword]
	c.state[keyword]++

	newAddr := c.genAddr(mask[:8], st)
	oldAddr := c.genAddr(mask[:8], st-1)
	endAddr := c.genAddr(mask[:8], c.getClientRoundEnd(round))

	//生成字典键值对
	node := append(util.Xor(oldAddr, newAddr), cipherValue...) //32+
	endNode := append(util.Xor(endAddr, newAddr), c.genDummy(mask[:16])...)

	//todo 满了，增加轮数

	return []*cemm.AddToken{{Addr: newAddr, Node: node}, {Addr: endAddr, Node: endNode}}

}
func (c *EMMClient) genGetToken(mask []byte, round int) *GetToken {
	addr := c.genAddr(mask[:8], c.getRoundEnd(round))
	return &GetToken{addr}
}
func (c *EMMClient) genDummy(aesKey []byte) []byte {
	dummy, err := util.AESEncryptCBC(aesKey, []byte("dummy"))
	if err != nil {
		log.Fatal(err)
	}
	return dummy
}
func (c *EMMClient) genNextRoundDummyTokens(stKey, dummy []byte, round int) (tokens []*cemm.AddToken) {
	sts := c.getAllClientRoundEnd(round + 1)
	leftAddr := c.genAddr(stKey, c.getRoundEnd(round))
	var rightAddr []byte
	var node []byte
	for _, st := range sts {
		rightAddr = c.genAddr(stKey, st)
		node = append(util.Xor(leftAddr, rightAddr), dummy...)
		tokens = append(tokens, &cemm.AddToken{Addr: slices.Clone(rightAddr), Node: slices.Clone(node)})
		leftAddr = rightAddr
	}
	return
}

func (c *EMMClient) Add(keyword, value string) error {

	mask := c.genKeywordMask(keyword) //关键字指纹，同时也作为关键字的子密钥

	tokens := make([]*cemm.AddToken, 0, 2)
	var round int
	if rly, err := c.stub.GetOrIncRound(context.Background(), &cemm.RoundRequest{Op: false, Tag: mask[16:]}); err != nil {
		return err
	} else {
		round = int(rly.Round)
	}
	if round == 0 {
		tokens = append(tokens, c.genNextRoundDummyTokens(mask[:8], c.genDummy(mask[:16]), 0)...)
		round = 1
	}
	tokens = append(tokens, c.genAddToken(keyword, value, round, mask)...)
	if _, err := c.stub.Add(context.Background(), &cemm.AddRequest{Tokens: tokens, Tag: mask[16:], Round: int32(round)}); err != nil {
		return err
	}
	log.Printf("Add keyword %s value %s in round %d\n", keyword, value, round)
	return nil
}

func (c *EMMClient) Get(keyword string) ([]string, int, error) {
	mask := c.genKeywordMask(keyword)

	var round int
	if rly, err := c.stub.GetOrIncRound(context.Background(), &cemm.RoundRequest{Op: true, Tag: mask[16:]}); err != nil {
		log.Printf("RPC ERROR: GetRound fail %v\n", err)
		return nil, 0, err
	} else {
		round = int(rly.Round)
	}
	//重置客户端状态
	c.state[keyword] = 1
	
	//初始化下一轮dummy
	dummy := c.genDummy(mask[:16])
	dtks := c.genNextRoundDummyTokens(mask[:8], dummy, round)
	token := c.genGetToken(mask, round)
	log.Printf("Get keyword %s  in round %d with addr %v \n", keyword, round, token.LastAddr[0])
	var res []string
	if stream, err := c.stub.Get(context.Background(), &cemm.GetRequest{Addr: token.LastAddr, Tag: mask[16:], Round: int32(round), DummyTokens: dtks}); err == nil {

		for {
			recv, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, 0, err
			}
			var plaintext []byte
			plaintext, err = util.AESDecryptCBC(mask[:16], recv.Node)
			if err != nil {
				log.Printf("解密失败：%v\n", err)
				return nil, 0, err
			}
			text := string(plaintext)
			if strings.Contains(text, "dummy") {
				continue
			}
			res = append(res, text)
		}

	} else {
		return nil, 0, err
	}

	return res, round, nil
}
func (c *EMMClient) Init(data [][]string, length int) error {
	tagList := make([][]byte, 0, len(data))
	initTokens := make([]*cemm.AddToken, 0, length+1+c.cnt)
	var err error
	var dt []byte
	for _, list := range data {
		k := list[0]
		mask := c.genKeywordMask(k)
		dummy := c.genDummy(mask[:16])
		tagList = append(tagList, mask[16:])
		var newAddr, oldAddr []byte
		var firstRealNode, node []byte
		oldAddr = c.genAddr(mask[8:16], 1) //real部分使用8到16位
		dt, err = util.AESEncryptCBC(mask[:16], []byte(list[1]))
		if err != nil {
			log.Printf("加密失败：%v\n", err)
			return err
		}
		firstRealNode = append(oldAddr, dt...)
		initTokens = append(initTokens, &cemm.AddToken{Addr: slices.Clone(oldAddr), Node: firstRealNode})
		for i := 2; i < len(list); i++ {
			newAddr = c.genAddr(mask[8:16], i)
			dt, err = util.AESEncryptCBC(mask[:16], []byte(list[i]))
			if err != nil {
				log.Printf("加密失败：%v\n", err)
				return err
			}
			node = append(util.Xor(oldAddr, newAddr), dt...)
			initTokens = append(initTokens, &cemm.AddToken{Addr: slices.Clone(newAddr), Node: slices.Clone(node)})
			oldAddr = newAddr
		}
		//最后一个初始位置作为0号位置的前继
		st0 := c.genAddr(mask[:8], 0)
		node0 := append(util.Xor(st0, newAddr), dummy...)
		initTokens = append(initTokens, &cemm.AddToken{Addr: st0, Node: node0})
		//初始化第一轮的dummy node
		initTokens = append(initTokens, c.genNextRoundDummyTokens(mask[:8], dummy, 0)...)

		//leftAddr := newAddr
		//var rightAddr []byte
		//for i := 0; i < c.limit; i += c.volume {
		//	rightAddr = c.genAddr(mask[:8], i)
		//	node = append(util.Xor(leftAddr, rightAddr), dummy...)
		//	initTokens = append(initTokens, &cemm.AddToken{Addr: slices.Clone(rightAddr), Node: slices.Clone(node)})
		//	leftAddr = rightAddr
		//}
	}
	//初始化关键字空间
	_, err = c.stub.InitTagSets(context.Background(), &cemm.InitTagSetsRequest{Tags: tagList})
	if err != nil {
		log.Printf("初始化关键字空间失败：%v\n", err)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, errSteam := c.stub.InitEDB(ctx)
	defer stream.CloseSend()
	if errSteam != nil {
		return errSteam
	}
	for _, tk := range initTokens {
		if errSend := stream.Send(&cemm.InitEDBRequest{Token: tk}); errSend != nil {
			if errors.Is(errSend, io.EOF) {
				return nil
			}
			log.Printf("初始化EDB失败：%v\n", errSend)
			return errSend
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil

}
