package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"happyagent/internal/jsonfile"
)

type AgentStatus string

const (
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusFailed   AgentStatus = "failed"
	AgentStatusComplete AgentStatus = "complete"
)

type Agent struct {
	ID        string      `json:"id"`         // ID is the stable teammate identifier used in mailbox routing.
	Name      string      `json:"name"`       // Name is the human-readable teammate label shown to the lead.
	Role      string      `json:"role"`       // Role describes the teammate's responsibility for prompts and traces.
	Status    AgentStatus `json:"status"`     // Status reports the latest lifecycle state.
	CreatedAt time.Time   `json:"created_at"` // CreatedAt is set when the agent is first registered.
	UpdatedAt time.Time   `json:"updated_at"` // UpdatedAt changes whenever state changes.
}

type AgentRun struct {
	ID          string    `json:"id"`                     // ID identifies one child run.
	AgentID     string    `json:"agent_id"`               // AgentID links the run to its teammate.
	Prompt      string    `json:"prompt"`                 // Prompt is the task given to the child agent.
	Output      string    `json:"output,omitempty"`       // Output is the child final answer or failure summary.
	TracePath   string    `json:"trace_path,omitempty"`   // TracePath points at the serialized child trace when available.
	Status      string    `json:"status"`                 // Status is completed or failed.
	StartedAt   time.Time `json:"started_at"`             // StartedAt records when child execution began.
	CompletedAt time.Time `json:"completed_at,omitempty"` // CompletedAt records when child execution finished.
	Error       string    `json:"error,omitempty"`        // Error stores the child failure message.
}

type MailboxMessage struct {
	ID         string    `json:"id"`                    // ID uniquely identifies this mailbox entry.
	From       string    `json:"from"`                  // From is the sender agent id.
	To         string    `json:"to"`                    // To is the recipient agent id, usually lead.
	Kind       string    `json:"kind"`                  // Kind is result, note, permission_request, or shutdown_ack.
	Content    string    `json:"content"`               // Content is the message body consumed by the lead.
	RunID      string    `json:"run_id,omitempty"`      // RunID links the message to a child run when applicable.
	Consumed   bool      `json:"consumed"`              // Consumed is true after the lead routes this message.
	CreatedAt  time.Time `json:"created_at"`            // CreatedAt preserves mailbox ordering.
	ConsumedAt time.Time `json:"consumed_at,omitempty"` // ConsumedAt records lead consumption time.
}

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	base := filepath.Join(root, ".happyagent", "agents")
	for _, dir := range []string{base, filepath.Join(base, "agents"), filepath.Join(base, "runs"), filepath.Join(base, "mailbox")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create agents store: %w", err)
		}
	}
	return &Store{root: base}, nil
}

func (s *Store) UpsertAgent(agent Agent) (Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	agent.ID = safeID(agent.ID)
	if agent.ID == "" {
		agent.ID = fmt.Sprintf("agent-%d", now.UnixNano())
	}
	if strings.TrimSpace(agent.Name) == "" {
		agent.Name = agent.ID
	}
	if agent.Status == "" {
		agent.Status = AgentStatusRunning
	}
	if existing, err := s.getAgentLocked(agent.ID); err == nil {
		agent.CreatedAt = existing.CreatedAt
	} else {
		agent.CreatedAt = now
	}
	agent.UpdatedAt = now
	return agent, jsonfile.Write(s.agentPath(agent.ID), agent)
}

func (s *Store) AppendRun(run AgentRun) (AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	run.ID = safeID(run.ID)
	if run.ID == "" {
		run.ID = fmt.Sprintf("run-%d", now.UnixNano())
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if run.CompletedAt.IsZero() && run.Status != "running" {
		run.CompletedAt = now
	}
	return run, jsonfile.Write(s.runPath(run.ID), run)
}

func (s *Store) AppendMessage(message MailboxMessage) (MailboxMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	message.ID = safeID(message.ID)
	if message.ID == "" {
		message.ID = fmt.Sprintf("msg-%d", now.UnixNano())
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	return message, jsonfile.Write(s.messagePath(message.ID), message)
}

func (s *Store) ListUnconsumed(to string) ([]MailboxMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages, err := s.listMessagesLocked()
	if err != nil {
		return nil, err
	}
	var out []MailboxMessage
	for _, message := range messages {
		if message.Consumed {
			continue
		}
		if to != "" && message.To != to {
			continue
		}
		out = append(out, message)
	}
	return out, nil
}

func (s *Store) MarkConsumed(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, id := range ids {
		message, err := s.getMessageLocked(id)
		if err != nil {
			return err
		}
		message.Consumed = true
		message.ConsumedAt = now
		if err := jsonfile.Write(s.messagePath(id), message); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) TracePath(runID string) string {
	return filepath.Join(s.root, "runs", safeID(runID)+".trace.json")
}

func (s *Store) getAgentLocked(id string) (Agent, error) {
	var agent Agent
	data, err := os.ReadFile(s.agentPath(id))
	if err != nil {
		return Agent{}, err
	}
	return agent, json.Unmarshal(data, &agent)
}

func (s *Store) getMessageLocked(id string) (MailboxMessage, error) {
	var message MailboxMessage
	data, err := os.ReadFile(s.messagePath(id))
	if err != nil {
		return MailboxMessage{}, err
	}
	return message, json.Unmarshal(data, &message)
}

func (s *Store) listMessagesLocked() ([]MailboxMessage, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "mailbox"))
	if err != nil {
		return nil, err
	}
	var out []MailboxMessage
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		message, err := s.getMessageLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) agentPath(id string) string {
	return filepath.Join(s.root, "agents", safeID(id)+".json")
}

func (s *Store) runPath(id string) string {
	return filepath.Join(s.root, "runs", safeID(id)+".json")
}

func (s *Store) messagePath(id string) string {
	return filepath.Join(s.root, "mailbox", safeID(id)+".json")
}

func safeID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), ".-")
}
