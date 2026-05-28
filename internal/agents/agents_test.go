package agents

import "testing"

func TestMailboxAppendListAndMarkConsumed(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	first, err := store.AppendMessage(MailboxMessage{From: "child", To: "lead", Kind: "result", Content: "done"})
	if err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	if _, err := store.AppendMessage(MailboxMessage{From: "child", To: "other", Kind: "result", Content: "hidden"}); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}
	messages, err := store.ListUnconsumed("lead")
	if err != nil {
		t.Fatalf("ListUnconsumed() error = %v", err)
	}
	if len(messages) != 1 || messages[0].ID != first.ID {
		t.Fatalf("unexpected messages: %+v", messages)
	}
	if err := store.MarkConsumed([]string{first.ID}); err != nil {
		t.Fatalf("MarkConsumed() error = %v", err)
	}
	messages, err = store.ListUnconsumed("lead")
	if err != nil {
		t.Fatalf("ListUnconsumed() after consume error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected consumed mailbox to be empty: %+v", messages)
	}
}
