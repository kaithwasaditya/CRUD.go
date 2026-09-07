package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// ---------- Data model + SQLite store ----------

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			done BOOLEAN NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}

	_, err = s.db.Exec(
		"INSERT INTO tasks (title, done) VALUES (?, ?), (?, ?), (?, ?)",
		"Buy milk", false,
		"Write README", false,
		"Learn Go", true,
	)
	return err
}

func (s *Store) list() ([]Task, error) {
	rows, err := s.db.Query("SELECT id, title, done FROM tasks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Done); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) get(id int) (Task, bool, error) {
	var task Task
	err := s.db.QueryRow("SELECT id, title, done FROM tasks WHERE id = ?", id).Scan(&task.ID, &task.Title, &task.Done)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return task, err == nil, err
}

func (s *Store) create(title string) (Task, error) {
	result, err := s.db.Exec("INSERT INTO tasks (title, done) VALUES (?, ?)", title, false)
	if err != nil {
		return Task{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Task{}, err
	}
	return Task{ID: int(id), Title: title, Done: false}, nil
}

func (s *Store) update(id int, title string, done bool) (Task, bool, error) {
	result, err := s.db.Exec("UPDATE tasks SET title = ?, done = ? WHERE id = ?", title, done, id)
	if err != nil {
		return Task{}, false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Task{}, false, err
	}
	if updated == 0 {
		return Task{}, false, nil
	}
	return Task{ID: id, Title: title, Done: done}, true, nil
}

func (s *Store) delete(id int) (bool, error) {
	result, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	deleted, err := result.RowsAffected()
	return deleted > 0, err
}

// ---------- JSON helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func idFromPath(w http.ResponseWriter, path string) (int, bool) {
	idText := strings.TrimPrefix(path, "/tasks/")
	if idText == "" || strings.Contains(idText, "/") {
		writeErr(w, http.StatusNotFound, "not found")
		return 0, false
	}
	id, err := strconv.Atoi(idText)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id must be a number")
		return 0, false
	}
	return id, true
}

// ---------- Handlers ----------

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "Task API",
		"version": "1.0",
		"endpoints": []string{
			"GET /tasks", "GET /tasks/{id}", "POST /tasks",
			"PUT /tasks/{id}", "DELETE /tasks/{id}",
		},
	})
}

func makeTasksHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks, err := store.list()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}

			writeJSON(w, http.StatusOK, tasks)

		case http.MethodPost:
			var body struct {
				Title string `json:"title"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
			title := strings.TrimSpace(body.Title)
			if title == "" {
				writeErr(w, http.StatusBadRequest, "title is required")
				return
			}
			created, err := store.create(title)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			writeJSON(w, http.StatusCreated, created)

		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func makeTaskByIDHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idFromPath(w, r.URL.Path)
		if !ok {
			return
		}

		switch r.Method {
		case http.MethodGet:
			t, found, err := store.get(id)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			if !found {
				writeErr(w, http.StatusNotFound, "Task not found")
				return
			}
			writeJSON(w, http.StatusOK, t)

		case http.MethodPut:
			var body struct {
				Title string `json:"title"`
				Done  bool   `json:"done"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
			title := strings.TrimSpace(body.Title)
			if title == "" {
				writeErr(w, http.StatusBadRequest, "title is required")
				return
			}
			updated, found, err := store.update(id, title, body.Done)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			if !found {
				writeErr(w, http.StatusNotFound, "Task not found")
				return
			}
			writeJSON(w, http.StatusOK, updated)

		case http.MethodDelete:
			deleted, err := store.delete(id)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			if !deleted {
				writeErr(w, http.StatusNotFound, "Task not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func newMux(store *Store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		rootHandler(w, r)
	})
	mux.HandleFunc("/tasks", makeTasksHandler(store))
	mux.HandleFunc("/tasks/", makeTaskByIDHandler(store))
	return mux
}

func main() {
	store, err := NewStore("tasks.db")
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer store.db.Close()

	mux := newMux(store)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Task API listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
