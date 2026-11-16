package service

import (
	"errors"
	"testing"

	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/cache"
	"github.com/mirainya/Prism/pkg/config"
	"gorm.io/gorm"
)

func TestLastEnabledAdministratorCannotBeRemoved(t *testing.T) {
	for _, operation := range []string{"demote", "disable"} {
		t.Run(operation, func(t *testing.T) {
			db := setupTestDB(t)
			admin := model.User{Username: "only-admin", Role: model.UserRoleAdmin, Status: 1, SessionVersion: 9}
			if err := db.Create(&admin).Error; err != nil {
				t.Fatal(err)
			}
			inactive := model.User{Username: "inactive-admin", Role: model.UserRoleAdmin, Status: 1}
			if err := db.Create(&inactive).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&inactive).Update("status", 0).Error; err != nil {
				t.Fatal(err)
			}
			deleted := model.User{Username: "deleted-admin", Role: model.UserRoleAdmin, Status: 1}
			if err := db.Create(&deleted).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Delete(&deleted).Error; err != nil {
				t.Fatal(err)
			}
			service := NewUserService()
			var err error
			if operation == "demote" {
				err = service.UpdateUserRole(admin.ID, model.UserRoleUser)
			} else {
				err = service.UpdateUserStatus(admin.ID, 0)
			}
			if !errors.Is(err, ErrLastAdministrator) {
				t.Fatalf("error=%v, want last-administrator rejection", err)
			}
			var current model.User
			if err := db.First(&current, admin.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Status != 1 || current.Role != model.UserRoleAdmin || current.SessionVersion != 9 {
				t.Fatalf("rejected operation changed user: %#v", current)
			}
		})
	}
}

func TestUserSuspensionPreservesIndividualKeyStates(t *testing.T) {
	db := setupTestDB(t)
	previousCache := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = previousCache })
	user := model.User{Username: "suspended-owner", Role: model.UserRoleUser, Status: 1, SessionVersion: 3}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []model.Token{
		{UserID: user.ID, Selector: "enabled", Status: 1, AuthVersion: 5},
		{UserID: user.ID, Selector: "disabled", Status: 1, AuthVersion: 8},
	} {
		if err := db.Create(&token).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.Token{}).Where("selector = ?", "disabled").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	service := NewUserService()
	for index, status := range []int8{0, 1} {
		if err := service.UpdateUserStatus(user.ID, status); err != nil {
			t.Fatal(err)
		}
		var current model.User
		if err := db.First(&current, user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.Status != status || current.SessionVersion != uint64(4+index) {
			t.Fatalf("user transition: %#v", current)
		}
		var tokens []model.Token
		if err := db.Where("user_id = ?", user.ID).Order("id").Find(&tokens).Error; err != nil {
			t.Fatal(err)
		}
		if len(tokens) != 2 || tokens[0].Status != 1 || tokens[0].AuthVersion != 5 || tokens[1].Status != 0 || tokens[1].AuthVersion != 8 {
			t.Fatalf("owner transition changed individual keys: %#v", tokens)
		}
	}
	if err := service.UpdateUserStatus(user.ID, 1); err != nil {
		t.Fatal(err)
	}
	var current model.User
	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.SessionVersion != 5 {
		t.Fatal("repeating the same status must not revoke sessions again")
	}
}

func TestAdministratorRemovalRequiresAnotherEnabledAdministrator(t *testing.T) {
	db := setupTestDB(t)
	previousCache := cache.Client
	cache.Client = nil
	t.Cleanup(func() { cache.Client = previousCache })
	first := model.User{Username: "admin-one", Role: model.UserRoleAdmin, Status: 1}
	second := model.User{Username: "admin-two", Role: model.UserRoleAdmin, Status: 1}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	service := NewUserService()
	if err := service.UpdateUserRole(first.ID, model.UserRoleUser); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateUserStatus(second.ID, 0); !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("error=%v, want protection after another admin was demoted", err)
	}
	if err := service.UpdateUserStatus(9999, 0); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing user error=%v", err)
	}
	if err := service.UpdateUserStatus(first.ID, 2); !errors.Is(err, ErrInvalidUserAccess) {
		t.Fatalf("invalid status error=%v", err)
	}
	if err := service.UpdateUserRole(first.ID, "superuser"); !errors.Is(err, ErrInvalidUserAccess) {
		t.Fatalf("invalid role error=%v", err)
	}
}

func TestRegistrationRequiresExplicitConfiguration(t *testing.T) {
	previous := config.Get()
	t.Cleanup(func() { config.C = previous })
	service := NewUserService()
	request := &RegisterRequest{Username: "new-member", Password: "test-password"}
	for _, cfg := range []*config.Config{nil, {}} {
		config.C = cfg
		if _, err := service.Register(request); !errors.Is(err, ErrRegistrationDisabled) {
			t.Fatalf("disabled registration error=%v", err)
		}
	}
	setupTestDB(t)
	config.C = &config.Config{Server: config.ServerConfig{RegistrationEnabled: true}}
	user, err := service.Register(request)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != model.UserRoleUser || user.Status != 1 || user.Password == request.Password {
		t.Fatal("registration must create an ordinary user with a hashed password")
	}
}
