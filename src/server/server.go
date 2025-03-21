package server

import (
	"DRW/src/rpc/cemm"
	"DRW/src/util"
	"context"
	"errors"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type EMMServer struct {
	cemm.UnimplementedCEMMServer
	//泄漏搜索模式
	round     map[string]int32           //有多个读者和写着并发访问，用锁保证其一致性
	writing   map[string][]*atomic.Int32 //有锁初始化，后续都是只读
	readStart map[string]*atomic.Int32   //有锁初始化，后续都是只读
	mutex     sync.Mutex
	edb       *badger.DB
}

func (s *EMMServer) Get(in *cemm.GetRequest, stream grpc.ServerStreamingServer[cemm.GetReply]) error {

	//todo 某一轮Get出错，后续Get将被永久阻塞。可以加入超时控制，让原子状态恢复正常
	//思路：当前轮已经处于半可读状态，且等待上一轮已经超时，则可以将当前轮置为全可读状态。需要cas，更大则更新

	//查询开始前先初始化下一轮的dummy node
	for _, tk := range in.DummyTokens {
		err := s.write(tk.Addr, tk.Node)
		if err != nil {
			log.Printf("EDB write dummy batch error: %v", err)
			return err
		}
	}

	stag := string(in.Tag)
	round := in.Round
	counter := s.writing[stag][int(round%10)]
	readStart := s.readStart[stag]
	tc := time.NewTicker(50 * time.Millisecond).C
	timeout := time.After(2 * time.Millisecond)
loop:
	for {
		select {
		case <-tc:
			if counter.Load() == 0 && readStart.CompareAndSwap(round, round+1) {
				break loop
			}
		case <-timeout:
			for {
				cur := readStart.Load()
				if cur > round {
					break loop
				} else {
					if readStart.CompareAndSwap(cur, round+1) {
						break loop
					}
				}
			}
		}
	}
	var data []byte
	addr := in.Addr
	for {
		value, err := s.read(addr)
		if err != nil {
			log.Printf("EDB read  error: %v", err)
			return err
		}
		if value == nil {
			break
		}
		addr, data = parseNode(addr, value)
		err = stream.Send(&cemm.GetReply{Node: slices.Clone(data)})
		if err != nil {
			log.Printf("server send error: %v", err)
			return err
		}
	}
	return nil
}

func (s *EMMServer) InitTagSets(ctx context.Context, in *cemm.InitTagSetsRequest) (*emptypb.Empty, error) {
	for _, ctag := range in.Tags {
		stag := string(ctag)

		s.round[stag] = 1

		ati := &atomic.Int32{}
		ati.Store(1)
		s.readStart[stag] = ati

		s.writing[stag] = make([]*atomic.Int32, 10)
		for i := 0; i < 10; i++ {
			s.writing[stag][i] = &atomic.Int32{}
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *EMMServer) InitEDB(steam grpc.ClientStreamingServer[cemm.InitEDBRequest, emptypb.Empty]) error {
	for {
		recv, err := steam.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		tk := recv.Token
		err = s.write(tk.Addr, tk.Node)
		//err = s.mwrite(tk.Addr, tk.Node)
		if err != nil {
			log.Printf("EDB write  batch error: %v", err)
			return err
		}
	}
}

func (s *EMMServer) GetOrIncRound(ctx context.Context, in *cemm.RoundRequest) (*cemm.RoundReply, error) {
	//todo 安全和效率的tradeoff：每一个get请求先判断当前轮有没有更新，无更新则选择阻塞去等上一轮的读者的结果（single flight）
	stag := string(in.Tag)
	rly := &cemm.RoundReply{}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	//非原始关键字空间的新关键字结构初始化
	if s.round[stag] == 0 {
		if !in.Op {
			rly.Round = 0 //对于非原始关键字的第一次插入，返回轮数=0，以告知其该请求的客户端做第一轮dummy的初始化。因为这个函数是互斥访问，这个初始化操作最终只有一个客户端执行
		} else {
			rly.Round = 1
		}
		s.round[stag] = 1
	} else {
		rly.Round = s.round[stag]
	}

	if s.writing[stag] == nil {
		s.writing[stag] = make([]*atomic.Int32, 10)
		for i := 0; i < 10; i++ {
			s.writing[stag][i] = &atomic.Int32{}
		}
	}
	if s.readStart[stag] == nil {
		ati := &atomic.Int32{}
		ati.Store(1)
		s.readStart[stag] = ati
	}
	if in.Op {
		s.round[stag]++
	} else {
		s.writing[stag][int(s.round[stag]%10)].Add(1)
	}
	return rly, nil
}

func (s *EMMServer) Add(ctx context.Context, in *cemm.AddRequest) (*emptypb.Empty, error) {
	counter := s.writing[string(in.Tag)][int(in.Round%10)]

	//确保写入出错也将写计数减1
	defer counter.Add(-1)

	for _, tk := range in.Tokens {
		if err := s.write(tk.Addr, tk.Node); err != nil {
			log.Println("EDB write batch error:", err)
			return &emptypb.Empty{}, err
		}
	}
	return &emptypb.Empty{}, nil
}

func parseNode(addr, node []byte) (preAddr, data []byte) {
	preAddr = util.Xor(addr, node[:32])
	data = node[32:]
	return
}

func NewEMMServer(db *badger.DB) *EMMServer {
	s := &EMMServer{
		round:     make(map[string]int32),
		writing:   make(map[string][]*atomic.Int32),
		readStart: make(map[string]*atomic.Int32),
		edb:       db,
	}
	return s
}

func (s *EMMServer) write(key, value []byte) error {
	// 2. 写入数据
	return s.edb.Update(func(txn *badger.Txn) error {
		var err error
		err = txn.Set(key, value)
		if err != nil {
			return err
		}
		return nil
	})

}
func (s *EMMServer) read(key []byte) ([]byte, error) {
	// 3. 读取数据
	var value []byte
	return value, s.edb.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}
		if item != nil {
			value, err = item.ValueCopy(nil)
		}
		return err
	})
}
