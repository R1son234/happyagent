package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"happyagent/internal/jsonfile"
)

const (
	TargetMemory = "memory"
	TargetUser   = "user"

	EntryDelimiter = "\n§\n"

	DefaultMemoryCharLimit = 2200
	DefaultUserCharLimit   = 1400

	memoryFileName = "memory.json"
	userFileName   = "user.json"
)

type MemoryEntry struct {
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type LongTermStore struct {
	dir    string
	mu     sync.RWMutex
	memory []MemoryEntry
	user   []MemoryEntry
	limits map[string]int

	snapshotMemory string
	snapshotUser   string
	snapshotLoaded bool
}

func NewLongTermStore(dir string) *LongTermStore {
	s := &LongTermStore{
		dir:    dir,
		limits: map[string]int{TargetMemory: DefaultMemoryCharLimit, TargetUser: DefaultUserCharLimit},
	}
	s.loadFromDisk()
	return s
}

func (s *LongTermStore) LoadSnapshot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshotMemory = s.renderBlock(TargetMemory, s.memory)
	s.snapshotUser = s.renderBlock(TargetUser, s.user)
	s.snapshotLoaded = true
}

func (s *LongTermStore) SnapshotText() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.snapshotLoaded {
		return ""
	}
	var parts []string
	if s.snapshotMemory != "" {
		parts = append(parts, s.snapshotMemory)
	}
	if s.snapshotUser != "" {
		parts = append(parts, s.snapshotUser)
	}
	return strings.Join(parts, "\n\n")
}

func (s *LongTermStore) SnapshotLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLoaded
}

func (s *LongTermStore) Save(target, content, source string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content cannot be empty")
	}
	if err := scanForThreats(content); err != nil {
		return err
	}
	if !isValidTarget(target) {
		return fmt.Errorf("invalid target %q: use %q or %q", target, TargetMemory, TargetUser)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entriesFor(target)
	limit := s.limits[target]

	for _, e := range entries {
		if e.Content == content {
			return nil
		}
	}

	newEntry := MemoryEntry{
		Content:   content,
		Source:    source,
		CreatedAt: time.Now(),
	}
	newEntries := append(entries, newEntry)
	newTotal := charCount(newEntries)
	if newTotal > limit {
		current := charCount(entries)
		return &CapacityError{
			Target:      target,
			Current:     current,
			Limit:       limit,
			Entries:     entries,
			AddingChars: utf8.RuneCountInString(content),
		}
	}

	s.setEntries(target, newEntries)
	return s.saveToDisk(target)
}

func (s *LongTermStore) Delete(target, oldText string) error {
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return fmt.Errorf("old_text cannot be empty")
	}
	if !isValidTarget(target) {
		return fmt.Errorf("invalid target %q: use %q or %q", target, TargetMemory, TargetUser)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entriesFor(target)
	var matches []int
	for i, e := range entries {
		if strings.Contains(e.Content, oldText) {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("no entry matched %q", oldText)
	}
	if len(matches) > 1 {
		unique := map[string]struct{}{}
		for _, idx := range matches {
			unique[entries[idx].Content] = struct{}{}
		}
		if len(unique) > 1 {
			previews := make([]string, 0, len(matches))
			for _, idx := range matches {
				c := entries[idx].Content
				if len(c) > 80 {
					c = c[:80] + "..."
				}
				previews = append(previews, c)
			}
			return &MultipleMatchError{OldText: oldText, Matches: previews}
		}
	}

	idx := matches[0]
	remaining := make([]MemoryEntry, 0, len(entries)-1)
	remaining = append(remaining, entries[:idx]...)
	remaining = append(remaining, entries[idx+1:]...)
	s.setEntries(target, remaining)
	return s.saveToDisk(target)
}

func (s *LongTermStore) List(target string) []MemoryEntry {
	if !isValidTarget(target) {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.entriesFor(target)
	out := make([]MemoryEntry, len(entries))
	copy(out, entries)
	return out
}

func (s *LongTermStore) Recall(query string, limit int) []MemoryEntry {
	if limit <= 0 {
		limit = 5
	}
	terms := tokenizeQuery(query)
	if len(terms) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		entry MemoryEntry
		score int
	}
	var results []scored

	for _, e := range s.memory {
		if sc := matchScore(e.Content, terms); sc > 0 {
			results = append(results, scored{entry: e, score: sc})
		}
	}
	for _, e := range s.user {
		if sc := matchScore(e.Content, terms); sc > 0 {
			results = append(results, scored{entry: e, score: sc})
		}
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}
	out := make([]MemoryEntry, len(results))
	for i, r := range results {
		out[i] = r.entry
	}
	return out
}

func (s *LongTermStore) CharCount(target string) int {
	if !isValidTarget(target) {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return charCount(s.entriesFor(target))
}

func (s *LongTermStore) CharLimit(target string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limits[target]
}

func (s *LongTermStore) AllEntries(target string) []MemoryEntry {
	return s.List(target)
}

// --- internal ---

func (s *LongTermStore) entriesFor(target string) []MemoryEntry {
	if target == TargetUser {
		return s.user
	}
	return s.memory
}

func (s *LongTermStore) setEntries(target string, entries []MemoryEntry) {
	if target == TargetUser {
		s.user = entries
	} else {
		s.memory = entries
	}
}

func (s *LongTermStore) loadFromDisk() {
	s.memory = readEntries(filepath.Join(s.dir, memoryFileName))
	s.user = readEntries(filepath.Join(s.dir, userFileName))
}

func (s *LongTermStore) saveToDisk(target string) error {
	var entries []MemoryEntry
	var filename string
	if target == TargetUser {
		entries = s.user
		filename = userFileName
	} else {
		entries = s.memory
		filename = memoryFileName
	}
	return jsonfile.Write(filepath.Join(s.dir, filename), entries)
}

func (s *LongTermStore) renderBlock(target string, entries []MemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	limit := s.limits[target]
	content := joinEntries(entries)
	current := utf8.RuneCountInString(content)
	pct := 0
	if limit > 0 {
		pct = current * 100 / limit
		if pct > 100 {
			pct = 100
		}
	}

	var header string
	if target == TargetUser {
		header = fmt.Sprintf("USER (user profile) [%d%% — %d/%d chars]", pct, current, limit)
	} else {
		header = fmt.Sprintf("MEMORY (agent notes) [%d%% — %d/%d chars]", pct, current, limit)
	}
	separator := strings.Repeat("═", 46)
	return separator + "\n" + header + "\n" + separator + "\n" + content
}

func readEntries(path string) []MemoryEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []MemoryEntry
	if err := jsonUnmarshal(data, &entries); err != nil {
		return nil
	}
	return entries
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func joinEntries(entries []MemoryEntry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = e.Content
	}
	return strings.Join(parts, EntryDelimiter)
}

func charCount(entries []MemoryEntry) int {
	return utf8.RuneCountInString(joinEntries(entries))
}

func isValidTarget(target string) bool {
	return target == TargetMemory || target == TargetUser
}

func tokenizeQuery(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".,:;!?()[]{}\"'")
		if utf8.RuneCountInString(f) >= 2 {
			terms = append(terms, f)
		}
	}
	return terms
}

func matchScore(content string, terms []string) int {
	lowered := strings.ToLower(content)
	score := 0
	for _, term := range terms {
		score += strings.Count(lowered, term)
	}
	return score
}

// --- security scanning ---

var threatPatterns = []struct {
	Pattern *regexp.Regexp
	ID      string
}{
	{regexp.MustCompile(`(?i)ignore\s+(previous|all|above|prior)(\s+\w+)*\s+instructions`), "prompt_injection"},
	{regexp.MustCompile(`(?i)you\s+are\s+now\s+`), "role_hijack"},
	{regexp.MustCompile(`(?i)do\s+not\s+tell\s+the\s+user`), "deception_hide"},
	{regexp.MustCompile(`(?i)system\s+prompt\s+override`), "sys_prompt_override"},
	{regexp.MustCompile(`(?i)disregard\s+(your|all|any)\s+(instructions|rules|guidelines)`), "disregard_rules"},
	{regexp.MustCompile(`(?i)curl\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL)`), "exfil_curl"},
	{regexp.MustCompile(`(?i)wget\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL)`), "exfil_wget"},
	{regexp.MustCompile(`(?i)cat\s+[^\n]*(\.env|credentials|\.netrc|\.pgpass)`), "read_secrets"},
	{regexp.MustCompile(`authorized_keys`), "ssh_backdoor"},
}

var invisibleChars = map[rune]bool{
	0x200B: true, // zero width space
	0x200C: true, // zero width non-joiner
	0x200D: true, // zero width joiner
	0x2060: true, // word joiner
	0xFEFF: true, // BOM
	0x202A: true, // left-to-right embedding
	0x202B: true, // right-to-left embedding
	0x202C: true, // pop directional formatting
	0x202D: true, // left-to-right override
	0x202E: true, // right-to-left override
}

func scanForThreats(content string) error {
	for _, r := range content {
		if invisibleChars[r] {
			return fmt.Errorf("blocked: content contains invisible unicode character U+%04X (possible injection)", r)
		}
	}
	for _, tp := range threatPatterns {
		if tp.Pattern.MatchString(content) {
			return fmt.Errorf("blocked: content matches threat pattern %q", tp.ID)
		}
	}
	return nil
}

// --- error types ---

type CapacityError struct {
	Target      string
	Current     int
	Limit       int
	Entries     []MemoryEntry
	AddingChars int
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf(
		"memory at %d/%d chars. Adding this entry (%d chars) would exceed the limit. Remove or replace existing entries first.",
		e.Current, e.Limit, e.AddingChars,
	)
}

type MultipleMatchError struct {
	OldText string
	Matches []string
}

func (e *MultipleMatchError) Error() string {
	return fmt.Sprintf("multiple entries matched %q. Be more specific.", e.OldText)
}
