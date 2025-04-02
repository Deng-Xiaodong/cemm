package client

import (
	"DRW/src/config"
	"DRW/src/rpc/cemm"
	"DRW/src/utlils"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
)

type EMMClient struct {
	cnt, volume, limit int
	idx                int
	state              map[string]*roundCount
	//方案暂时假定后续添加不能超过初始化的关键字空间
	stub cemm.CEMMClient
}
type roundCount struct {
	round int
	count int
}

func NewEMMClient(idx int, cf *config.Config, stub cemm.CEMMClient) *EMMClient {
	return &EMMClient{
		cnt:    cf.ClientCnt,
		volume: cf.Volume,
		limit:  cf.Limit,
		idx:    idx,
		state:  make(map[string]*roundCount),
		stub:   stub,
	}
}

func (c *EMMClient) getClientRoundStart(round int) int {
	return c.cnt*c.volume*(round-1) + c.volume*(c.idx-1) + 1
}
func (c *EMMClient) getClientRoundEnd(round int) int {
	return c.cnt*c.volume*(round-1) + c.volume*c.idx
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
	return utlils.H1(string(utlils.H2(st)))
}
func (c *EMMClient) genKeywordMask(keyword string) []byte {
	return utlils.H1(keyword)
}
func (c *EMMClient) genAddToken(keyword, value string, round int, mask []byte) (next, preNext *cemm.AddToken) {

	cipherValue, err := utlils.AESEncryptCBC(mask2key(mask), []byte(value))
	if err != nil {
		log.Fatalln(err)
	}
	//获取关键字计数
	rc := c.state[keyword]
	if rc == nil {
		rc = &roundCount{}
		c.state[keyword] = rc
	}
	if rc.round < round {
		rc.round = round
		rc.count = c.getClientRoundStart(rc.round)
	}
	st := rc.count
	rc.count++

	newAddr := c.genAddr(mask2st(mask), st)
	oldAddr := c.genAddr(mask2st(mask), st-1)
	endAddr := c.genAddr(mask2st(mask), c.getClientRoundEnd(round))

	//生成字典键值对
	zero := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(zero, 0)
	node := append(zero, append(utlils.Xor(oldAddr, newAddr), cipherValue...)...) //4+32+32
	endNode := append(zero, append(utlils.Xor(endAddr, newAddr), c.genDummy(mask2key(mask))...)...)

	//todo 满了，增加轮数

	return &cemm.AddToken{Addr: newAddr, Node: node}, &cemm.AddToken{Addr: endAddr, Node: endNode}

}

func (c *EMMClient) genGetToken(mask []byte, round int) []byte {
	return c.genAddr(mask2st(mask), c.getRoundEnd(round))
}

func (c *EMMClient) genDummy(aesKey []byte) []byte {
	dummy, err := utlils.AESEncryptCBC(aesKey, []byte("dummy"))
	if err != nil {
		log.Fatal(err)
	}
	return dummy
}

//func (c *EMMClient) genNextRoundDummyTokens(stKey, dummy []byte, round int) (tokens []*cemm.AddToken) {
//	sts := c.getAllClientRoundEnd(round + 1)
//	leftAddr := c.genAddr(stKey, c.getRoundEnd(round))
//	var rightAddr []byte
//	var node []byte
//	for _, st := range sts {
//		rightAddr = c.genAddr(stKey, st)
//		node = append(utlils.Xor(leftAddr, rightAddr), dummy...)
//		tokens = append(tokens, &cemm.AddToken{Addr: slices.Clone(rightAddr), Node: slices.Clone(node)})
//		leftAddr = rightAddr
//	}
//	return
//}

func (c *EMMClient) Add(keyword, value string) error {

	mask := c.genKeywordMask(keyword) //关键字指纹，同时也作为关键字的子密钥

	var round int
	if rly, err := c.stub.GetOrIncRound(context.Background(), &cemm.RoundRequest{Op: false, Tag: mask2tag(mask)}); err != nil {
		return err
	} else {
		round = int(rly.Round)
	}

	next, preNext := c.genAddToken(keyword, value, round, mask)
	if _, err := c.stub.Add(context.Background(), &cemm.AddRequest{Next: next, PreNext: preNext}); err != nil {
		log.Printf("RPC ERROR: Add failed: %v", err)
		return err
	}
	return nil
}

func (c *EMMClient) Get(keyword string) ([]string, error) {
	mask := c.genKeywordMask(keyword)

	var round int
	if rly, err := c.stub.GetOrIncRound(context.Background(), &cemm.RoundRequest{Op: true, Tag: mask2tag(mask)}); err != nil {
		log.Printf("RPC ERROR: GetRound fail %v\n", err)
		return nil, err
	} else {
		round = int(rly.Round)
	}

	gtk := c.genGetToken(mask, round)
	var res []string

	//测试用
	res = append(res, strconv.Itoa(round))

	if stream, err := c.stub.Get(context.Background(), &cemm.GetRequest{Addr: gtk}); err == nil {
		for {
			recv, errRecv := stream.Recv()
			if errRecv != nil {
				if errors.Is(errRecv, io.EOF) {
					break
				}
				log.Printf("RPC ERROR: Get fail %v\n", errRecv)
				return nil, errRecv
			}
			var plaintext []byte
			plaintext, err = utlils.AESDecryptCBC(mask2key(mask), recv.Node)
			if err != nil {
				log.Printf("解密失败：%v\n", err)
				return nil, err
			}
			text := string(plaintext)
			if strings.Contains(text, "dummy") {
				continue
			}
			res = append(res, text)
		}

	} else {
		return nil, err
	}
	return res, nil
}
func (c *EMMClient) Init(keySet []string) error {

	initTokens := make([]*cemm.AddToken, 0, c.limit/c.volume+2)
	tagSet := make([][]byte, 0, len(keySet))
	for _, l := range keySet {

		mask := c.genKeywordMask(l)
		tagSet = append(tagSet, mask2tag(mask))
		dummy := c.genDummy(mask2key(mask))
		zero := make([]byte, 4, 4)
		binary.BigEndian.PutUint32(zero, 0)

		st0 := c.genAddr(mask2st(mask), 0)
		node0 := append(zero, append(st0, dummy...)...)
		initTokens = append(initTokens, &cemm.AddToken{Addr: slices.Clone(st0), Node: node0})

		var newAddr, oldAddr []byte
		var node []byte
		oldAddr = slices.Clone(st0)
		for i := 1; i*c.volume <= c.limit; i++ {
			newAddr = c.genAddr(mask2st(mask), i*c.volume)
			node = append(zero, append(utlils.Xor(oldAddr, newAddr), dummy...)...)
			initTokens = append(initTokens, &cemm.AddToken{Addr: slices.Clone(newAddr), Node: slices.Clone(node)})
			oldAddr = newAddr
		}
	}
	_, err := c.stub.Init(context.Background(), &cemm.InitRequest{Tags: tagSet, Dummys: initTokens})
	if err != nil {
		log.Printf("Init fail %v\n", err)
		return err
	}
	log.Println("Init success")
	return nil

}

func mask2tag(mask []byte) []byte {
	return mask[16:]
}
func mask2st(mask []byte) []byte {
	return mask[:8]
}
func mask2key(mask []byte) []byte {
	return mask[:16]
}
