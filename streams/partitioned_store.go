package streams

import (
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

type changeLogPartition[T StateStore] struct {
	store T
	topic string
}

type changeLogData[T any] struct {
	store T
	topic string
}

func (sp *changeLogPartition[T]) grab() T {
	return sp.store
}

// we just need to untype this generic contstarint from StateStore to any
// to eas up some type gymnastics downstream
func (sp *changeLogPartition[T]) changeLogData() *changeLogData[T] {
	return &changeLogData[T]{
		store: sp.store,
		topic: sp.topic,
	}
}

func (sp *changeLogPartition[T]) release() {
}

func (sp *changeLogPartition[T]) receiveChangeInternal(record *kgo.Record) {
	// this is only called during partition prep, so locking is not necessary
	// this will improve performance a bit
	sp.store.ReceiveChange(newIncomingRecord(record))
}

func (sp *changeLogPartition[T]) revokedInternal() {
	sp.grab().Revoked()
	sp.release()
}

type TopicPartitionCallback[T any] func(TopicPartition) T
type partitionedChangeLog[T StateStore] struct {
	data           map[int32]*changeLogPartition[T]
	factory        TopicPartitionCallback[T]
	changeLogTopic string
	mux            sync.Mutex
}

func newPartitionedChangeLog[T StateStore](factory TopicPartitionCallback[T], changeLogTopic string) *partitionedChangeLog[T] {
	return &partitionedChangeLog[T]{
		changeLogTopic: changeLogTopic,
		data:           make(map[int32]*changeLogPartition[T]),
		factory:        factory}
}

func (ps *partitionedChangeLog[T]) Len() int {
	return len(ps.data)
}

// func (ps *partitionedChangeLog[T]) GetStore(ec *EventContext[T]) (sp *changeLogPartition[T], ok bool) {
// 	return ps.getStore(ec.TopicPartition().Partition)
// }

func (ps *partitionedChangeLog[T]) getStore(partition int32) (sp *changeLogPartition[T], ok bool) {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	sp, ok = ps.data[partition]
	return
}

func (ps *partitionedChangeLog[T]) assign(partition int32) *changeLogPartition[T] {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	var ok bool
	var sp *changeLogPartition[T]
	log.Debugf("PartitionedStore assigning %d", partition)
	if sp, ok = ps.data[partition]; !ok {
		sp = &changeLogPartition[T]{
			store: ps.factory(TopicPartition{partition, ps.changeLogTopic}),
			topic: ps.changeLogTopic,
		}
		ps.data[partition] = sp
	}
	return sp
}

func (ps *partitionedChangeLog[T]) revoke(partition int32) {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	log.Debugf("PartitionedStore revoking %d", partition)
	if store, ok := ps.data[partition]; ok {
		delete(ps.data, partition)
		store.revokedInternal()
	}
}
