package storage

import (
    "context"
    "tunesday/internal/core"
)

// Store abstracts persistence backend.
type Store interface {
    Load(ctx context.Context) (*core.Data, error)
    Save(ctx context.Context, d *core.Data) error
}
