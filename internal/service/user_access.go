package service

import (
	"errors"

	"github.com/mirainya/Prism/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLastAdministrator = errors.New("at least one enabled administrator is required")
	ErrInvalidUserAccess = errors.New("invalid user role or status")
)

func (s *UserService) updateUserAccess(userID uint, role *model.UserRole, status *int8) error {
	if userID == 0 || role != nil && *role != model.UserRoleAdmin && *role != model.UserRoleUser ||
		status != nil && *status != 0 && *status != 1 {
		return ErrInvalidUserAccess
	}
	changed := false
	err := model.DB().Transaction(func(tx *gorm.DB) error {
		// Lock administrators in the same order for both operations, so concurrent
		// demotions and suspensions cannot each remove the last administrator.
		var administrators []model.User
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role = ? AND status = 1", model.UserRoleAdmin).Order("id").Find(&administrators).Error; err != nil {
			return err
		}
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		nextRole, nextStatus := user.Role, user.Status
		if role != nil {
			nextRole = *role
		}
		if status != nil {
			nextStatus = *status
		}
		if nextRole == user.Role && nextStatus == user.Status {
			return nil
		}
		if user.Role == model.UserRoleAdmin && user.Status == 1 &&
			(nextRole != model.UserRoleAdmin || nextStatus != 1) && len(administrators) < 2 {
			return ErrLastAdministrator
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
			"role": nextRole, "status": nextStatus,
			"session_version": gorm.Expr("session_version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		changed = true
		return nil
	})
	if err == nil && changed {
		revokeUserSessions(userID)
	}
	return err
}
