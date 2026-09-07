package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type memoryRepository struct {
	tasks []Task
}

func (r *memoryRepository) List() ([]Task, error) { return r.tasks, nil }

func (r *memoryRepository) Get(id int) (Task, bool, error) {
	for _, task := range r.tasks {
		if task.ID == id {
			return task, true, nil
		}
	}
	return Task{}, false, nil
}

func (r *memoryRepository) Create(title string) (Task, error) {
	task := Task{ID: len(r.tasks) + 1, Title: title}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *memoryRepository) Update(id int, title string, done bool) (Task, bool, error) {
	for index, task := range r.tasks {
		if task.ID == id {
			r.tasks[index].Title = title
			r.tasks[index].Done = done
			return r.tasks[index], true, nil
		}
	}
	return Task{}, false, nil
}

func (r *memoryRepository) Delete(id int) (bool, error) {
	for index, task := range r.tasks {
		if task.ID == id {
			r.tasks = append(r.tasks[:index], r.tasks[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func TestRoutesWorkWithRepository(t *testing.T) {
	repository := &memoryRepository{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/tasks", http.NoBody)
	newMux(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("empty POST status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestResetEndpointIsNotRegistered(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/reset", nil)
	newMux(&memoryRepository{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST /reset status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
