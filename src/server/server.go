package server

import (
	"DRW/src/rpc/cemm"
	"DRW/src/util"
	"context"
	"errors"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"log"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

type EMMServer struct {
	cemm.UnimplementedCEMMServer
	round map[string]*atomic.Int32
	locks map[string]*sync.Mutex
	stash map[string]map[string]struct{}
	edb   *badger.DB
}

func (s *EMMServer) Init(ctx context.Context, in *cemm.InitRequest) (*emptypb.Empty, error) {
	//初始化round、stash、locks，后续无锁使用
	for _, tag := range in.Tags {
		stag := string(tag)

		//round
		at := &atomic.Int32{}
		at.Store(1)
		s.round[stag] = at

		//stash
		s.stash[stag] = make(map[string]struct{})

		//locks
		s.locks[stag] = new(sync.Mutex)

	}
	var err error
	for _, dummy := range in.Dummys {
		err = s.write(dummy.Addr, dummy.Node)
		if err != nil {
			log.Printf("write dummy failed: %v", err)
			return nil, err
		}
	}
	return nil, nil
}

func (s *EMMServer) Get(in *cemm.GetRequest, stream grpc.ServerStreamingServer[cemm.GetReply]) error {

	stag := string(in.Tag)
	cur := make(map[string]struct{})
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
		} else {
			cur[string(addr)] = struct{}{}
		}

		addr, data = parseNode(addr, value)
		err = stream.Send(&cemm.GetReply{Node: slices.Clone(data)})
		if err != nil {
			log.Printf("server send error: %v", err)
			return err
		}
	}
	mu := s.locks[stag]
	mu.Lock() //有可能更后面的查询轮先拿到锁，前面轮的结果会被填充后面轮真实值和dummy值
	diff := s.unionAndGetDiff(stag, cur)
	mu.Unlock()

	for addr := range maps.Keys(diff) {
		val, err := s.read([]byte(addr))
		if err != nil {
			log.Printf("EDB read  error: %v", err)
			return err
		}
		if val == nil {
			log.Fatal("EDB read  empty for diff")
		}
		err = stream.Send(&cemm.GetReply{Node: slices.Clone(val[32:])})
		if err != nil {
			log.Printf("server send error: %v", err)
			return err
		}
	}
	log.Printf("diff len: %d", len(diff))
	return nil
}

func (s *EMMServer) GetOrIncRound(ctx context.Context, in *cemm.RoundRequest) (*cemm.RoundReply, error) {
	stag := string(in.Tag)
	rly := &cemm.RoundReply{}
	t, ok := s.round[stag]
	if !ok || t == nil {
		log.Println("EDB get round error: init error")
		return rly, errors.New("init error")
	}
	switch in.Op {
	case true:
		//查询
		for {
			old := t.Load()
			if t.CompareAndSwap(old, old+1) {
				rly.Round = old
				break
			}
		}
	case false:
		//添加
		rly.Round = t.Load()
	}
	return rly, nil
}

func (s *EMMServer) Add(ctx context.Context, in *cemm.AddRequest) (*emptypb.Empty, error) {
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
		edb:   db,
		round: make(map[string]*atomic.Int32),
		stash: make(map[string]map[string]struct{}),
		locks: make(map[string]*sync.Mutex),
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

func (s *EMMServer) unionAndGetDiff(stag string, cur map[string]struct{}) map[string]struct{} {
	pre := s.stash[stag]
	ret := make(map[string]struct{})
	diff := make(map[string]struct{})
	maps.Copy(ret, cur)
	for k, _ := range pre {
		if _, ok := cur[k]; !ok {
			ret[k] = struct{}{}
			diff[k] = struct{}{}
		}
	}
	s.stash[stag] = ret
	return diff
}
