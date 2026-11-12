package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// Reader is the read-only view of domain state a builder needs. store.Store
// supplies all of it but the queue; internal/core composes the two.
type Reader interface {
	Chats() []domain.Chat
	GetChat(id int64) (domain.Chat, bool)
	// Runs returns a history's runs, oldest first: the unbroken stretches it
	// is stored as. A window is built inside one of them.
	Runs(h domain.HistoryKey) []domain.Run
	FolderFilters() []domain.FolderFilter
	// Topics and Topic answer for a forum's topics (#275).
	Topics(chatID int64) []domain.Topic
	Topic(chatID int64, topicID int) (domain.Topic, bool)
	// Outbox returns a history's queued sends in submission order. It is not on
	// store.Store because the queue is not part of the account cache: the
	// composite reader in internal/core supplies it (#193).
	Outbox(h domain.HistoryKey) []domain.OutboxEntry
	// Account is where the account stands. It is not on store.Store either:
	// the owner holds it, and none of it outlives the owner (#309).
	Account() AccountContents
}
