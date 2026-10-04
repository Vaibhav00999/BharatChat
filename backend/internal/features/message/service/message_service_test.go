// backend/internal/features/message/service/message_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/bharatchat/backend/internal/features/message/domain"
	"github.com/bharatchat/backend/internal/features/message/service"
	"github.com/stretchr/testify/require"
)

type fakeMessageRepo struct {
	messages map[string]*domain.Message
	statuses map[string]map[string]domain.MessageStatus // messageID -> userID -> status
	nextID   int
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{
		messages: map[string]*domain.Message{},
		statuses: map[string]map[string]domain.MessageStatus{},
	}
}

func (f *fakeMessageRepo) Create(ctx context.Context, msg domain.Message, recipientUserIDs []string) (*domain.Message, error) {
	f.nextID++
	id := fmt_Sprintf(f.nextID)
	msg.ID = id
	f.messages[id] = &msg

	f.statuses[id] = map[string]domain.MessageStatus{}
	for _, r := range recipientUserIDs {
		f.statuses[id][r] = domain.MessageStatus{MessageID: id, UserID: r, State: domain.StateSent}
	}

	return &msg, nil
}

func fmt_Sprintf(n int) string {
	digits := "0123456789"
	if n == 0 {
		return "msg-0"
	}
	s := ""
	for n > 0 {
		s = string(digits[n%10]) + s
		n /= 10
	}
	return "msg-" + s
}

func (f *fakeMessageRepo) ListHistory(ctx context.Context, chatID string, limit int, beforeMessageID *string) ([]domain.Message, error) {
	var out []domain.Message
	for _, m := range f.messages {
		if m.ChatID == chatID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *fakeMessageRepo) FindByID(ctx context.Context, messageID string) (*domain.Message, error) {
	m, ok := f.messages[messageID]
	if !ok {
		return nil, nil
	}
	return m, nil
}

var stateRankTest = map[domain.DeliveryState]int{domain.StateSent: 0, domain.StateDelivered: 1, domain.StateRead: 2}

func (f *fakeMessageRepo) UpdateStatus(ctx context.Context, messageID, userID string, newState domain.DeliveryState) (bool, error) {
	statusMap, ok := f.statuses[messageID]
	if !ok {
		return false, nil
	}
	current, ok := statusMap[userID]
	if !ok {
		return false, nil
	}
	if stateRankTest[newState] <= stateRankTest[current.State] {
		return false, nil
	}
	current.State = newState
	statusMap[userID] = current
	return true, nil
}

func (f *fakeMessageRepo) MarkAllReadUpTo(ctx context.Context, chatID, userID, upToMessageID string) ([]string, error) {
	var changed []string
	for messageID, statusMap := range f.statuses {
		m, ok := f.messages[messageID]
		if !ok || m.ChatID != chatID {
			continue
		}
		status, ok := statusMap[userID]
		if !ok || status.State == domain.StateRead {
			continue
		}
		status.State = domain.StateRead
		statusMap[userID] = status
		changed = append(changed, messageID)
	}
	return changed, nil
}

func (f *fakeMessageRepo) GetStatusesForMessage(ctx context.Context, messageID string) ([]domain.MessageStatus, error) {
	var out []domain.MessageStatus
	for _, s := range f.statuses[messageID] {
		out = append(out, s)
	}
	return out, nil
}

type allowingChatAuth struct{ shouldFail bool }

func (a *allowingChatAuth) AuthorizeParticipant(ctx context.Context, chatID, userID string) error {
	if a.shouldFail {
		return require_ErrorForbidden()
	}
	return nil
}

func require_ErrorForbidden() error {
	return &testForbiddenError{}
}

type testForbiddenError struct{}

func (e *testForbiddenError) Error() string { return "forbidden" }

type fixedParticipantLister struct{ ids []string }

func (f *fixedParticipantLister) ListOtherParticipantIDs(ctx context.Context, chatID, excludingUserID string) ([]string, error) {
	return f.ids, nil
}

type recordingPublisher struct {
	events []publishedEvent
}

type fixedReadReceiptPolicy struct{ enabled bool }

func (p *fixedReadReceiptPolicy) ReadReceiptsEnabled(context.Context, string) (bool, error) {
	return p.enabled, nil
}

type publishedEvent struct {
	userID       string
	envelopeType string
	payload      interface{}
}

func (p *recordingPublisher) PublishToUser(ctx context.Context, userID string, envelopeType string, payload interface{}) error {
	p.events = append(p.events, publishedEvent{userID: userID, envelopeType: envelopeType, payload: payload})
	return nil
}

func TestSendMessage_PersistsAndPublishesToRecipients(t *testing.T) {
	repo := newFakeMessageRepo()
	chatAuth := &allowingChatAuth{}
	participants := &fixedParticipantLister{ids: []string{"user-b", "user-c"}}
	publisher := &recordingPublisher{}

	svc := service.NewMessageService(repo, chatAuth, participants, publisher)

	result, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID:   "chat-1",
		SenderID: "user-a",
		Type:     domain.MessageTypeText,
		Body:     "hello there",
	})

	require.NoError(t, err)
	require.Equal(t, "hello there", *result.Message.Body)
	require.ElementsMatch(t, []string{"user-b", "user-c"}, result.RecipientUserIDs)
	require.Len(t, publisher.events, 2)
	require.Equal(t, "new_message", publisher.events[0].envelopeType)
}

func TestSendMessage_EmptyTextBody_Fails(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := service.NewMessageService(repo, &allowingChatAuth{}, &fixedParticipantLister{}, &recordingPublisher{})

	_, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "   ",
	})
	require.Error(t, err)
}

func TestSendMessage_ProductionModeRejectsPlaintext(t *testing.T) {
	svc := service.NewMessageService(newFakeMessageRepo(), &allowingChatAuth{}, &fixedParticipantLister{}, &recordingPublisher{})
	svc.RequireEncryption(true)

	_, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "plaintext",
	})

	require.Error(t, err)
}

func TestSendMessage_AcceptsOpaqueEncryptedPayload(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := service.NewMessageService(repo, &allowingChatAuth{}, &fixedParticipantLister{ids: []string{"user-b"}}, &recordingPublisher{})
	svc.RequireEncryption(true)
	deviceID := "device-a"

	result, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText,
		Ciphertext: make([]byte, 32), EncryptionProtocol: "signal",
		EncryptionVersion: 1, SenderDeviceID: &deviceID,
	})

	require.NoError(t, err)
	require.Nil(t, result.Message.Body)
	require.Equal(t, deviceID, *result.Message.SenderDeviceID)
}

func TestSendMessage_RejectsMediaUntilAttachmentPipelineExists(t *testing.T) {
	svc := service.NewMessageService(newFakeMessageRepo(), &allowingChatAuth{}, &fixedParticipantLister{}, &recordingPublisher{})

	_, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeImage,
	})

	require.Error(t, err)
}

func TestSendMessage_RejectsReplyTargetFromAnotherChat(t *testing.T) {
	repo := newFakeMessageRepo()
	repo.messages["other-message"] = &domain.Message{ID: "other-message", ChatID: "chat-2"}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, &fixedParticipantLister{}, &recordingPublisher{})
	replyID := "other-message"

	_, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText,
		Body: "reply", ReplyToMessageID: &replyID,
	})

	require.Error(t, err)
}

func TestSendMessage_NotAParticipant_Forbidden(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := service.NewMessageService(repo, &allowingChatAuth{shouldFail: true}, &fixedParticipantLister{}, &recordingPublisher{})

	_, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "hi",
	})
	require.Error(t, err)
}

func TestMarkDelivered_NotifiesOriginalSender(t *testing.T) {
	repo := newFakeMessageRepo()
	participants := &fixedParticipantLister{ids: []string{"user-b"}}
	publisher := &recordingPublisher{}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, participants, publisher)

	sent, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "ping",
	})
	require.NoError(t, err)

	publisher.events = nil // reset after the send-time publish, to isolate the delivery ack below

	err = svc.MarkDelivered(context.Background(), sent.Message.ID, "user-b")
	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, "user-a", publisher.events[0].userID)
	require.Equal(t, "delivery_ack", publisher.events[0].envelopeType)
}

func TestMarkDelivered_RedundantCall_DoesNotDoubleNotify(t *testing.T) {
	repo := newFakeMessageRepo()
	participants := &fixedParticipantLister{ids: []string{"user-b"}}
	publisher := &recordingPublisher{}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, participants, publisher)

	sent, _ := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "ping",
	})

	require.NoError(t, svc.MarkDelivered(context.Background(), sent.Message.ID, "user-b"))
	publisher.events = nil

	require.NoError(t, svc.MarkDelivered(context.Background(), sent.Message.ID, "user-b"))
	require.Empty(t, publisher.events, "a redundant delivered ack must not re-notify the sender")
}

func TestMarkRead_AfterDelivered_Succeeds(t *testing.T) {
	repo := newFakeMessageRepo()
	participants := &fixedParticipantLister{ids: []string{"user-b"}}
	publisher := &recordingPublisher{}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, participants, publisher)

	sent, _ := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "ping",
	})

	require.NoError(t, svc.MarkDelivered(context.Background(), sent.Message.ID, "user-b"))
	publisher.events = nil

	require.NoError(t, svc.MarkRead(context.Background(), sent.Message.ID, "user-b"))
	require.Len(t, publisher.events, 1)
	require.Equal(t, "read_ack", publisher.events[0].envelopeType)
}

func TestMarkRead_WhenReceiptsDisabled_UpdatesStateWithoutPublishing(t *testing.T) {
	repo := newFakeMessageRepo()
	participants := &fixedParticipantLister{ids: []string{"user-b"}}
	publisher := &recordingPublisher{}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, participants, publisher)
	svc.UseReadReceiptPolicy(&fixedReadReceiptPolicy{enabled: false})

	sent, err := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "private read",
	})
	require.NoError(t, err)
	publisher.events = nil

	require.NoError(t, svc.MarkRead(context.Background(), sent.Message.ID, "user-b"))
	require.Equal(t, domain.StateRead, repo.statuses[sent.Message.ID]["user-b"].State)
	require.Empty(t, publisher.events)
}

func TestMarkChatReadUpTo_NotifiesOncePerAffectedMessage(t *testing.T) {
	repo := newFakeMessageRepo()
	participants := &fixedParticipantLister{ids: []string{"user-b"}}
	publisher := &recordingPublisher{}
	svc := service.NewMessageService(repo, &allowingChatAuth{}, participants, publisher)

	first, _ := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "one",
	})
	second, _ := svc.SendMessage(context.Background(), service.SendMessageInput{
		ChatID: "chat-1", SenderID: "user-a", Type: domain.MessageTypeText, Body: "two",
	})
	publisher.events = nil

	err := svc.MarkChatReadUpTo(context.Background(), "chat-1", "user-b", second.Message.ID)
	require.NoError(t, err)
	require.Len(t, publisher.events, 2)

	seenMessageIDs := map[string]bool{}
	for _, e := range publisher.events {
		require.Equal(t, "read_ack", e.envelopeType)
		payloadMap := e.payload.(map[string]interface{})
		seenMessageIDs[payloadMap["messageId"].(string)] = true
	}
	require.True(t, seenMessageIDs[first.Message.ID])
	require.True(t, seenMessageIDs[second.Message.ID])
}
