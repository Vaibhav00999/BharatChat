package service_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bharatchat/backend/internal/features/message/domain"
	"github.com/bharatchat/backend/internal/features/message/service"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func TestPrelaunchBlocksPlaintextAndCiphertextBeforeSideEffects(t *testing.T) {
	deviceID := "11111111-1111-4111-8111-111111111111"
	for _, input := range []service.SendMessageInput{
		{Type: domain.MessageTypeText, Body: "hello"},
		{Type: domain.MessageTypeText, Ciphertext: make([]byte, 32), EncryptionProtocol: "signal", EncryptionVersion: 1, SenderDeviceID: &deviceID},
	} {
		// No repository, authorizer, or publisher may be touched when locked.
		svc := service.NewMessageService(nil, nil, nil, nil)
		svc.SetMessagingEnabled(false)
		result, err := svc.SendMessage(context.Background(), input)
		require.Nil(t, result)
		appErr, ok := apperror.As(err)
		require.True(t, ok)
		require.Equal(t, http.StatusForbidden, appErr.HTTPStatus)
	}
}
