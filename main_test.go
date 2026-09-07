package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTasksPersistAfterReopeningDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "tasks.db")
	store, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	tasks, err := store.list()
	if err != nil {
		t.Fatalf("list seeded tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("seeded task count = %d, want 3", len(tasks))
	}

	created, err := store.create("Persists after restart")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.db.Close()

	task, found, err := reopened.get(created.ID)
	if err != nil {
		t.Fatalf("get persisted task: %v", err)
	}
	if !found || task.Title != created.Title {
		t.Fatalf("persisted task = %#v, found = %t", task, found)
	}
}

func TestResetEndpointIsNotRegistered(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.db.Close()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/reset", nil)
	newMux(store).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST /reset status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
