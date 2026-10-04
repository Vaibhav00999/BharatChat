package domain

import (
	"context"
	"io"
)

// AccountExportRepository streams a consistent account archive without forcing
// large message histories into process memory.
type AccountExportRepository interface {
	WriteAccountExport(ctx context.Context, userID string, destination io.Writer) (found bool, err error)
}
