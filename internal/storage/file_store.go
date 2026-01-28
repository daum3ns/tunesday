package storage

import (
    "context"
    "encoding/json"
    "errors"
    "os"

    "tunesday/internal/core"
)

type FileStore struct{ path string }

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (fs *FileStore) Load(ctx context.Context) (*core.Data, error) {
    f, err := os.Open(fs.path)
    if errors.Is(err, os.ErrNotExist) {
        return core.NewData(), nil
    }
    if err != nil {
        return nil, err
    }
    defer f.Close()
    dec := json.NewDecoder(f)
    var d core.Data
    if err := dec.Decode(&d); err != nil {
        return nil, err
    }
    if d.Participants == nil {
        d.Participants = map[string]int{}
    }
    return &d, nil
}

func (fs *FileStore) Save(ctx context.Context, d *core.Data) error {
    tmp := fs.path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(d); err != nil {
        f.Close()
        _ = os.Remove(tmp)
        return err
    }
    if err := f.Close(); err != nil {
        _ = os.Remove(tmp)
        return err
    }
    return os.Rename(tmp, fs.path)
}
