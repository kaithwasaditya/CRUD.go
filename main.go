package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ---------- Stage 2: data model + in-memory "database" ----------

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type Store struct {
	mu     sync.Mutex
	tasks  []Task
	nextID int
}

func NewStore() *Store {
	s := &Store{}
	s.reset()
	return s
}

// reset seeds the store back to its 3 example tasks (used at boot and by POST /reset)
func (s *Store) reset() {
	s.tasks = []Task{
		{ID: 1, Title: "Buy milk", Done: false},
		{ID: 2, Title: "Write README", Done: false},
		{ID: 3, Title: "Learn Go", Done: true},
	}
	s.nextID = 4
}

func (s *Store) list() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, len(s.tasks))
	copy(out, s.tasks)
	return out
}

func (s *Store) get(id int) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

func (s *Store) create(title string) Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.nextID, Title: title, Done: false}
	s.nextID++
	s.tasks = append(s.tasks, t)
	return t
}

func (s *Store) update(id int, title string, done bool) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks[i].Title = title
			s.tasks[i].Done = done
			return s.tasks[i], true
		}
	}
	return Task{}, false
}

func (s *Store) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return true
		}
	}
	return false
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

// idFromPath extracts the trailing /tasks/{id} segment and parses it as an int.
// Returns ok=false with the response already written if the path is malformed.
var taskIDPath = regexp.MustCompile(`^/tasks/([^/]+)$`)

func idFromPath(w http.ResponseWriter, path string) (int, bool) {
	m := taskIDPath.FindStringSubmatch(path)
	if m == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
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
			"GET /stats", "POST /reset",
		},
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func makeTasksHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks := store.list()

			// ★ extras: ?done=true and ?search=milk query params
			if doneParam := r.URL.Query().Get("done"); doneParam != "" {
				wantDone, err := strconv.ParseBool(doneParam)
				if err != nil {
					writeErr(w, http.StatusBadRequest, "done must be true or false")
					return
				}
				filtered := tasks[:0:0]
				for _, t := range tasks {
					if t.Done == wantDone {
						filtered = append(filtered, t)
					}
				}
				tasks = filtered
			}
			if search := r.URL.Query().Get("search"); search != "" {
				filtered := tasks[:0:0]
				needle := strings.ToLower(search)
				for _, t := range tasks {
					if strings.Contains(strings.ToLower(t.Title), needle) {
						filtered = append(filtered, t)
					}
				}
				tasks = filtered
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
			created := store.create(title)
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
			t, found := store.get(id)
			if !found {
				writeErr(w, http.StatusNotFound, "Task "+strconv.Itoa(id)+" not found")
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
			updated, found := store.update(id, title, body.Done)
			if !found {
				writeErr(w, http.StatusNotFound, "Task "+strconv.Itoa(id)+" not found")
				return
			}
			writeJSON(w, http.StatusOK, updated)

		case http.MethodDelete:
			if !store.delete(id) {
				writeErr(w, http.StatusNotFound, "Task "+strconv.Itoa(id)+" not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ★ extra: GET /stats
func makeStatsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks := store.list()
		done := 0
		for _, t := range tasks {
			if t.Done {
				done++
			}
		}
		writeJSON(w, http.StatusOK, map[string]int{
			"total": len(tasks),
			"done":  done,
			"open":  len(tasks) - done,
		})
	}
}

// ★ extra: POST /reset
func makeResetHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		store.mu.Lock()
		store.reset()
		store.mu.Unlock()
		writeJSON(w, http.StatusOK, store.list())
	}
}

func main() {
	store := NewStore()
	_ = sort.Strings // (kept import used if you extend sorting later)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		rootHandler(w, r)
	})
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/tasks", makeTasksHandler(store))
	mux.HandleFunc("/tasks/", makeTaskByIDHandler(store))
	mux.HandleFunc("/stats", makeStatsHandler(store))
	mux.HandleFunc("/reset", makeResetHandler(store))

	// Stage 5: Swagger UI + the OpenAPI spec it reads
	mux.Handle("/openapi.json", http.FileServer(http.Dir("static")))
	mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("static/swagger-ui"))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Task API listening on http://localhost:%s (docs at /docs/)", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
