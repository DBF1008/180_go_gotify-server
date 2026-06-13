package database

import (
	"fmt"

	"github.com/gotify/server/v2/model"
	"gorm.io/gorm"
)

// GetUserByName returns the user by the given name or nil.
func (d *GormDatabase) GetUserByName(name string) (*model.User, error) {
	user := new(model.User)
	err := d.DB.Where("name = ?", name).Find(user).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	if user.Name == name {
		return user, err
	}
	return nil, err
}

// GetUserByID returns the user by the given id or nil.
func (d *GormDatabase) GetUserByID(id uint) (*model.User, error) {
	user := new(model.User)
	err := d.DB.Find(user, id).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	if user.ID == id {
		return user, err
	}
	return nil, err
}

// CountUser returns the user count which satisfies the given condition.
func (d *GormDatabase) CountUser(condition ...interface{}) (int64, error) {
	c := int64(-1)
	handle := d.DB.Model(new(model.User))
	if len(condition) == 1 {
		handle = handle.Where(condition[0])
	} else if len(condition) > 1 {
		handle = handle.Where(condition[0], condition[1:]...)
	}
	err := handle.Count(&c).Error
	return c, err
}

// GetUsers returns all users.
func (d *GormDatabase) GetUsers() ([]*model.User, error) {
	var users []*model.User
	err := d.DB.Find(&users).Error
	return users, err
}

// DeleteUserByID deletes a user by its id together with all owned data in a
// single transaction. If any step fails the entire operation is rolled back so
// that no partial state is left behind.
func (d *GormDatabase) DeleteUserByID(id uint) error {
	return d.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Delete all messages belonging to the user's applications.
		if err := tx.Where(
			"application_id IN (SELECT id FROM applications WHERE user_id = ?)", id,
		).Delete(&model.Message{}).Error; err != nil {
			return fmt.Errorf("delete user %d messages: %w", id, err)
		}

		// 2. Delete all applications owned by the user.
		if err := tx.Where("user_id = ?", id).Delete(&model.Application{}).Error; err != nil {
			return fmt.Errorf("delete user %d applications: %w", id, err)
		}

		// 3. Delete all clients (including expired ones) for the user.
		if err := tx.Where("user_id = ?", id).Delete(&model.Client{}).Error; err != nil {
			return fmt.Errorf("delete user %d clients: %w", id, err)
		}

		// 4. Delete all plugin configurations for the user.
		if err := tx.Where("user_id = ?", id).Delete(&model.PluginConf{}).Error; err != nil {
			return fmt.Errorf("delete user %d plugin configs: %w", id, err)
		}

		// 5. Finally delete the user record itself.
		if err := tx.Where("id = ?", id).Delete(&model.User{}).Error; err != nil {
			return fmt.Errorf("delete user %d: %w", id, err)
		}

		return nil
	})
}

// UpdateUser updates a user.
func (d *GormDatabase) UpdateUser(user *model.User) error {
	return d.DB.Save(user).Error
}

// CreateUser creates a user.
func (d *GormDatabase) CreateUser(user *model.User) error {
	return d.DB.Create(user).Error
}
