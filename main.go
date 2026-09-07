package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// TaskRepository contains the database operations used by the API.
// An in-memory repository or a Postgres repository can both implement it.
type TaskRepository interface {
	List() ([]Task, error)
	Get(id int) (Task, bool, error)
	Create(title string) (Task, error)
	Update(id int, title string, done bool) (Task, bool, error)
	Delete(id int) (bool, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(databaseURL string) (*PostgresRepository, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresRepository{db: db}, nil
}

func (r *PostgresRepository) List() ([]Task, error) {
	rows, err := r.db.Query("SELECT id, title, done FROM tasks ORDER BY id")
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

func (r *PostgresRepository) Get(id int) (Task, bool, error) {
	var task Task
	err := r.db.QueryRow("SELECT id, title, done FROM tasks WHERE id = $1", id).Scan(&task.ID, &task.Title, &task.Done)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return task, err == nil, err
}

func (r *PostgresRepository) Create(title string) (Task, error) {
	var task Task
	err := r.db.QueryRow(
		"INSERT INTO tasks (title, done) VALUES ($1, $2) RETURNING id, title, done",
		title, false,
	).Scan(&task.ID, &task.Title, &task.Done)
	return task, err
}

func (r *PostgresRepository) Update(id int, title string, done bool) (Task, bool, error) {
	var task Task
	err := r.db.QueryRow(
		"UPDATE tasks SET title = $1, done = $2 WHERE id = $3 RETURNING id, title, done",
		title, done, id,
	).Scan(&task.ID, &task.Title, &task.Done)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return task, err == nil, err
}

func (r *PostgresRepository) Delete(id int) (bool, error) {
	var deletedID int
	err := r.db.QueryRow("DELETE FROM tasks WHERE id = $1 RETURNING id", id).Scan(&deletedID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeErr(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func taskID(w http.ResponseWriter, path string) (int, bool) {
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

func makeTasksHandler(repository TaskRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks, err := repository.List()
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
			task, err := repository.Create(title)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			writeJSON(w, http.StatusCreated, task)

		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func makeTaskHandler(repository TaskRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := taskID(w, r.URL.Path)
		if !ok {
			return
		}

		switch r.Method {
		case http.MethodGet:
			task, found, err := repository.Get(id)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			if !found {
				writeErr(w, http.StatusNotFound, "Task not found")
				return
			}
			writeJSON(w, http.StatusOK, task)

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
			task, found, err := repository.Update(id, title, body.Done)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "database error")
				return
			}
			if !found {
				writeErr(w, http.StatusNotFound, "Task not found")
				return
			}
			writeJSON(w, http.StatusOK, task)

		case http.MethodDelete:
			deleted, err := repository.Delete(id)
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

func newMux(repository TaskRepository) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", makeTasksHandler(repository))
	mux.HandleFunc("/tasks/", makeTaskHandler(repository))
	return mux
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	repository, err := NewPostgresRepository(databaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer repository.db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Task API listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newMux(repository)))
}
