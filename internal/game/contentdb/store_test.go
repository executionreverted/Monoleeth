package contentdb

import (
	"errors"
	"testing"
)

func TestNewStoreRejectsNilDB(t *testing.T) {
	store, err := NewStore(nil)

	if store != nil {
		t.Fatal("NewStore(nil) store != nil, want nil")
	}
	if !errors.Is(err, ErrNilDatabase) {
		t.Fatalf("NewStore(nil) error = %v, want ErrNilDatabase", err)
	}
}
