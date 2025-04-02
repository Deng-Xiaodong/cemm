package server

import (
	"DRW/src/rpc/cemm"
	"DRW/src/utlils"
	"context"
	"encoding/binary"
	"errors"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"log"
	"slices"
	"sync/atomic"
)

type AtomicCounter struct {
	value uint32
}

func NewAtomicCounter(v uint32) *AtomicCounter {
	return &AtomicCounter{v}
}
func (ac *AtomicCounter) Load() uint32 {
	return atomic.LoadUint32(&ac.value)
}
func (ac *AtomicCounter) FetchAndInc() uint32 {
	for {
		old := atomic.LoadUint32(&ac.value)
		if atomic.CompareAndSwapUint32(&ac.value, old, old+1) {
			return old
		}
	}
}

type EMMServer struct {
	cemm.UnimplementedCEMMServer

	cnt   *AtomicCounter
	round map[string]*AtomicCounter

	edb *badger.DB
}

func NewEMMServer(db *badger.DB) *EMMServer {
	s := &EMMServer{
		edb:   db,
		cnt:   NewAtomicCounter(1),
		round: make(map[string]*AtomicCounter),
	}
	return s
}

func (s *EMMServer) Init(ctx context.Context, in *cemm.InitRequest) (*emptypb.Empty, error) {

	for _, tag := range in.Tags {
		stag := string(tag)
		//round
		s.round[stag] = NewAtomicCounter(1)
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
	//进入线性化点
	countGet := s.cnt.FetchAndInc()

	var data []byte
	addr := in.Addr
	var count uint32
	for {

		value, err := s.read(addr)
		if err != nil {
			log.Printf("EDB read  error: %v", err)
			return err
		}
		if value == nil {
			break
		}

		//检查可见性
		count, addr, data = parseNode(addr, value)

		if count == 0 || count > countGet {
			//log.Printf("get miss node with count   %v", count)
			continue
		}

		err = stream.Send(&cemm.GetReply{Node: slices.Clone(data)})
		if err != nil {
			log.Printf("server send error: %v", err)
			return err
		}
	}
	return nil
}

func (s *EMMServer) GetOrIncRound(ctx context.Context, in *cemm.RoundRequest) (*cemm.RoundReply, error) {
	stag := string(in.Tag)
	t, ok := s.round[stag]
	if !ok || t == nil {
		log.Println("EDB get round error: init error")
		return nil, errors.New("init error")
	}

	rly := &cemm.RoundReply{}
	if in.Op {
		//查询
		rly.Round = t.FetchAndInc()
	} else {
		//添加
		rly.Round = t.Load()
	}
	return rly, nil
}

func (s *EMMServer) Add(ctx context.Context, in *cemm.AddRequest) (*emptypb.Empty, error) {

	err := s.write(in.Next.Addr, in.Next.Node)
	if err != nil {
		log.Println("EDB write next error:", err)
		return &emptypb.Empty{}, err
	}

	err = s.write(in.PreNext.Addr, in.PreNext.Node)
	if err != nil {
		log.Println("EDB write preNext error:", err)
		return &emptypb.Empty{}, err
	}

	fullNode(s.cnt.FetchAndInc(), in.Next.Node)
	//线性化点
	//log.Println("EDB add node with count :", binary.BigEndian.Uint32(in.Next.Node[:4]))
	for s.write(in.Next.Addr, in.Next.Node) != nil {
	}

	return &emptypb.Empty{}, nil
}

// 功能函数
func parseNode(addr, node []byte) (count uint32, preAddr, data []byte) {
	count = binary.BigEndian.Uint32(node[:4])
	preAddr = utlils.Xor(addr, node[4:36])
	data = node[36:]
	return
}
func fullNode(count uint32, node []byte) {
	binary.BigEndian.PutUint32(node[:4], count)
}

// db相关
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
