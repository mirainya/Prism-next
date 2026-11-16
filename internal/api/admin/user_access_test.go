package admin

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/cache"
	"gorm.io/gorm"
)

var userAccessTestID atomic.Uint64

func userAccessFixture(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:user_access_%d?mode=memory&cache=shared", userAccessTestID.Add(1))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	var old *gorm.DB
	if model.HasDB() {
		old = model.DB()
	}
	previousCache := cache.Client
	cache.Client = nil
	model.SetDB(db)
	t.Cleanup(func() {
		model.SetDB(old)
		cache.Client = previousCache
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.User{BaseModel: model.BaseModel{ID: 1}, Username: "only-admin", Role: model.UserRoleAdmin, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, uint(1))
		c.Set(middleware.ContextKeyUserRole, "admin")
	})
	router.Use(middleware.AdminOnly())
	router.PUT("/users/:id/status", UpdateUserStatus)
	router.PUT("/users/:id/role", UpdateUserRole)
	return router
}

func TestUserAccessEndpointsRejectMissingStatusAndLastAdministrator(t *testing.T) {
	router := userAccessFixture(t)
	for _, test := range []struct {
		path, body string
		status     int
	}{
		{"/users/1/status", `{}`, 400},
		{"/users/1/status", `{"status":null}`, 400},
		{"/users/1/status", `{"status":2}`, 400},
		{"/users/1/status", `{"status":0}`, 409},
		{"/users/1/role", `{"role":"user"}`, 409},
		{"/users/99/status", `{"status":0}`, 404},
		{"/users/1/status", `{"status":1}`, 200},
	} {
		request := httptest.NewRequest("PUT", test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s %s: status=%d body=%s", test.path, test.body, response.Code, response.Body)
		}
	}
}
