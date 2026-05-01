package observe

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type Event struct {
	Time    time.Time         `json:"time"`
	Type    string            `json:"type"`
	Message string            `json:"message"`
	Data    map[string]string `json:"data,omitempty"`
}

type Recorder struct {
	mu     sync.Mutex
	events []Event
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) Add(eventType string, message string, data map[string]string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Event{
		Time:    time.Now(),
		Type:    eventType,
		Message: message,
		Data:    cloneMap(data),
	})
}

func (r *Recorder) Events() []Event {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]Event, len(r.events))
	copy(events, r.events)
	return events
}

type Metrics struct {
	mu                sync.Mutex
	RunsTotal         int            `json:"runs_total"`
	RunSuccessTotal   int            `json:"run_success_total"`
	RunFailureTotal   int            `json:"run_failure_total"`
	StepsTotal        int            `json:"steps_total"`
	ToolCallsTotal    int            `json:"tool_calls_total"`
	ToolFailuresTotal int            `json:"tool_failures_total"`
	TokensTotal       int            `json:"tokens_total"`
	ErrorCategories   map[string]int `json:"error_categories"`
}

func NewMetrics() *Metrics {
	return &Metrics{ErrorCategories: map[string]int{}}
}

func (m *Metrics) RecordRun(success bool, stepCount int, toolCalls int, totalTokens int, errorCategory string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunsTotal++
	if success {
		m.RunSuccessTotal++
	} else {
		m.RunFailureTotal++
		if errorCategory != "" {
			m.ErrorCategories[errorCategory]++
		}
	}
	m.StepsTotal += stepCount
	m.ToolCallsTotal += toolCalls
	m.TokensTotal += totalTokens
}

func (m *Metrics) RecordToolFailure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToolFailuresTotal++
}

func (m *Metrics) Snapshot() Metrics {
	if m == nil {
		return Metrics{ErrorCategories: map[string]int{}}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := Metrics{
		RunsTotal:         m.RunsTotal,
		RunSuccessTotal:   m.RunSuccessTotal,
		RunFailureTotal:   m.RunFailureTotal,
		StepsTotal:        m.StepsTotal,
		ToolCallsTotal:    m.ToolCallsTotal,
		ToolFailuresTotal: m.ToolFailuresTotal,
		TokensTotal:       m.TokensTotal,
		ErrorCategories:   map[string]int{},
	}
	for key, value := range m.ErrorCategories {
		snapshot.ErrorCategories[key] = value
	}
	return snapshot
}

func (m Metrics) PrometheusText() string {
	return fmt.Sprintf(
		"happyagent_runs_total %d\nhappyagent_run_success_total %d\nhappyagent_run_failure_total %d\nhappyagent_steps_total %d\nhappyagent_tool_calls_total %d\nhappyagent_tool_failures_total %d\nhappyagent_tokens_total %d\n",
		m.RunsTotal,
		m.RunSuccessTotal,
		m.RunFailureTotal,
		m.StepsTotal,
		m.ToolCallsTotal,
		m.ToolFailuresTotal,
		m.TokensTotal,
	)
}

type Logger struct {
	writer io.Writer
}

func NewLogger(writer io.Writer) *Logger {
	return &Logger{writer: writer}
}

func (l *Logger) WriteEvent(event Event) error {
	if l == nil || l.writer == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = l.writer.Write(append(data, '\n'))
	return err
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
