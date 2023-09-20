package fsm

import (
	"encoding/json"
	"go-stateful-service/internal/model"
	"io"
	"log"
	"sync"

	"github.com/hashicorp/raft"
)

type FSM struct {
	mu sync.RWMutex
	kv map[string]string
}

type InternalFSM interface {
	raft.FSM
	Read(string) (string, bool)
}

var _ InternalFSM = (*FSM)(nil)

func NewFSM() InternalFSM {
	return &FSM{kv: make(map[string]string)}
}

func (f *FSM) Restore(snapshot io.ReadCloser) error {
	data, err := io.ReadAll(snapshot)
	if err != nil {
		return err
	}
	var snapshotData map[string]string
	if err := json.Unmarshal(data, &snapshotData); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.kv = make(map[string]string)
	for k, v := range snapshotData {
		f.kv[k] = v
	}

	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	snapshot := make(map[string]string)
	for k, v := range f.kv {
		snapshot[k] = v
	}

	return &FSMSnapshot{snapshot: snapshot}, nil
}

func (f *FSM) Apply(l *raft.Log) interface{} {
	if l.Type == raft.LogCommand {
		var kv model.KeyValue
		if err := json.Unmarshal(l.Data, &kv); err != nil {
			log.Printf("Failed to unmarshal log data: %v", err)
			return nil
		}

		f.mu.Lock()
		defer f.mu.Unlock()
		if kv.Value == nil {
			delete(f.kv, kv.Key)
		} else {
			f.kv[kv.Key] = *kv.Value
		}
	}

	return nil
}

func (f *FSM) Read(key string) (string, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	val, ok := f.kv[key]
	return val, ok
}
