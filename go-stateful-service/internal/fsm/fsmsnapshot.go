package fsm

import (
	"encoding/json"

	"github.com/hashicorp/raft"
)

type FSMSnapshot struct {
	snapshot map[string]string
}

var _ raft.FSMSnapshot = (*FSMSnapshot)(nil)

func (s *FSMSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s.snapshot)
	if err != nil {
		return err
	}

	if _, err := sink.Write(data); err != nil {
		return err
	}

	return sink.Close()
}

func (s *FSMSnapshot) Release() {
	s.snapshot = nil
}
