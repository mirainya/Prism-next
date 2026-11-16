package open

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func TestAudioTaskOwnershipAndCapabilityIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := repository.New(db)
	if err != nil {
		t.Fatal(err)
	}
	previous := unifiedVideoStore
	unifiedVideoStore = store
	t.Cleanup(func() { unifiedVideoStore = previous })
	now := time.Now().UTC()
	for _, item := range []struct {
		operation string
		status    int
		found     bool
	}{
		{"audio.generate", http.StatusOK, true},
		{"images.generate", http.StatusNotFound, true},
		{"audio.generate", http.StatusNotFound, false},
	} {
		rows := sqlmock.NewRows([]string{"id", "call_status", "task_status", "progress", "summary", "created_at", "updated_at"})
		if item.found {
			rows.AddRow(31, "in_progress", "queued", 0, []byte(`{"model":"public-voice","operation":"`+item.operation+`"}`), now, now)
		}
		mock.ExpectQuery("SELECT c.id,c.status,t.status,t.progress,t.parameter_summary,t.created_at,t.updated_at").
			WithArgs("audio-task", uint(11), uint(7)).WillReturnRows(rows)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/audio/tasks/audio-task", nil)
		c.Params = gin.Params{{Key: "id", Value: "audio-task"}}
		c.Set(middleware.ContextKeyToken, &model.Token{BaseModel: model.BaseModel{ID: 7}, UserID: 11})
		GetAudioTask(c)
		if recorder.Code != item.status {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if item.status == http.StatusOK && (!strings.Contains(recorder.Body.String(), `"status":"queued"`) || !strings.Contains(recorder.Body.String(), `"model":"public-voice"`)) {
			t.Fatalf("projection = %s", recorder.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAudioFailureDoesNotReadOrExposeProviderDiagnostics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := repository.New(db)
	if err != nil {
		t.Fatal(err)
	}
	previous := unifiedVideoStore
	unifiedVideoStore = store
	t.Cleanup(func() { unifiedVideoStore = previous })
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT c.id,c.status,t.status,t.progress,t.parameter_summary,t.created_at,t.updated_at").WithArgs("audio-task", uint(11), uint(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "call_status", "task_status", "progress", "summary", "created_at", "updated_at"}).AddRow(31, "failed", "failed", 0, []byte(`{"model":"public-voice","operation":"audio.generate"}`), now, now))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/audio/tasks/audio-task", nil)
	c.Params = gin.Params{{Key: "id", Value: "audio-task"}}
	c.Set(middleware.ContextKeyToken, &model.Token{BaseModel: model.BaseModel{ID: 7}, UserID: 11})
	GetAudioTask(c)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":"audio_task_failed"`) {
		t.Fatalf("failure: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAudioIngressRejectsUnauthenticatedAndPrivateReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, item := range []struct {
		body          string
		authenticated bool
		status        int
	}{
		{`{}`, false, http.StatusUnauthorized},
		{`{"model":"public-voice","input":"你好","reference_audio":"http://127.0.0.1/private.wav"}`, true, http.StatusBadRequest},
		{`{"model":"public-voice","input":"你好","reference_audio":"https://assets.example/voice.wav","speed":2}`, true, http.StatusBadRequest},
		{`{"model":"` + strings.Repeat("x", 65<<10) + `"}`, true, http.StatusRequestEntityTooLarge},
	} {
		router := gin.New()
		router.POST("/v1/audio/generations", func(c *gin.Context) {
			if item.authenticated {
				c.Set(middleware.ContextKeyToken, &model.Token{BaseModel: model.BaseModel{ID: 7}, UserID: 11})
			}
			CreateAudioGeneration(c)
		})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/audio/generations", strings.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != item.status {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
}
