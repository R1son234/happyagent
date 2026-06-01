package desktop

import (
	"sync"
	"time"
)

type RunEvent struct {
	Type      string    `json:"type"`
	Message   string    `json:"message,omitempty"`
	TaskName  string    `json:"task_name,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	Subtask   string    `json:"subtask,omitempty"`
	Status    string    `json:"status,omitempty"`
	StepIndex int       `json:"step_index,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type runEventStream struct {
	history []RunEvent
	subs    map[chan RunEvent]struct{}
	closed  bool
}

type runEventBroker struct {
	mu      sync.Mutex
	streams map[string]*runEventStream
}

func newRunEventBroker() *runEventBroker {
	return &runEventBroker{streams: map[string]*runEventStream{}}
}

func (b *runEventBroker) Subscribe(streamID string) ([]RunEvent, chan RunEvent, func()) {
	if b == nil || streamID == "" {
		return nil, nil, func() {}
	}
	b.mu.Lock()
	stream := b.streams[streamID]
	if stream == nil {
		stream = &runEventStream{subs: map[chan RunEvent]struct{}{}}
		b.streams[streamID] = stream
	}
	backlog := append([]RunEvent(nil), stream.history...)
	ch := make(chan RunEvent, 32)
	stream.subs[ch] = struct{}{}
	b.mu.Unlock()
	return backlog, ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		stream := b.streams[streamID]
		if stream == nil {
			return
		}
		delete(stream.subs, ch)
		close(ch)
		if stream.closed && len(stream.subs) == 0 {
			delete(b.streams, streamID)
		}
	}
}

func (b *runEventBroker) Publish(streamID string, event RunEvent) {
	if b == nil || streamID == "" {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	b.mu.Lock()
	stream := b.streams[streamID]
	if stream == nil {
		stream = &runEventStream{subs: map[chan RunEvent]struct{}{}}
		b.streams[streamID] = stream
	}
	stream.history = append(stream.history, event)
	if len(stream.history) > 200 {
		stream.history = append([]RunEvent(nil), stream.history[len(stream.history)-200:]...)
	}
	subs := make([]chan RunEvent, 0, len(stream.subs))
	for ch := range stream.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *runEventBroker) Finalize(streamID string) {
	if b == nil || streamID == "" {
		return
	}
	b.mu.Lock()
	stream := b.streams[streamID]
	if stream == nil {
		b.mu.Unlock()
		return
	}
	stream.closed = true
	shouldDelete := len(stream.subs) == 0
	b.mu.Unlock()
	if shouldDelete {
		b.mu.Lock()
		delete(b.streams, streamID)
		b.mu.Unlock()
		return
	}
	time.AfterFunc(2*time.Minute, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		stream := b.streams[streamID]
		if stream != nil && stream.closed && len(stream.subs) == 0 {
			delete(b.streams, streamID)
		}
	})
}
