package database

import (
	"github.com/gotify/server/v2/model"
	"gorm.io/gorm"
)

// GetMessageByID returns the messages for the given id or nil.
func (d *GormDatabase) GetMessageByID(id uint) (*model.Message, error) {
	msg := new(model.Message)
	err := d.DB.Find(msg, id).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	if msg.ID == id {
		return msg, err
	}
	return nil, err
}

// CreateMessage creates a message.
func (d *GormDatabase) CreateMessage(message *model.Message) error {
	return d.DB.Create(message).Error
}

// GetMessagesByUser returns all messages from a user.
func (d *GormDatabase) GetMessagesByUser(userID uint) ([]*model.Message, error) {
	var messages []*model.Message
	err := d.DB.Joins("JOIN applications ON applications.user_id = ?", userID).
		Where("messages.application_id = applications.id").Order("messages.id desc").Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// GetMessagesByUserSince returns limited messages from a user.
// If since is 0 it will be ignored.
func (d *GormDatabase) GetMessagesByUserSince(userID uint, limit int, since uint) ([]*model.Message, error) {
	var messages []*model.Message
	db := d.DB.Joins("JOIN applications ON applications.user_id = ?", userID).
		Where("messages.application_id = applications.id").Order("messages.id desc").Limit(limit)
	if since != 0 {
		db = db.Where("messages.id < ?", since)
	}
	err := db.Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// GetMessagesByApplication returns all messages from an application.
func (d *GormDatabase) GetMessagesByApplication(tokenID uint) ([]*model.Message, error) {
	var messages []*model.Message
	err := d.DB.Where("application_id = ?", tokenID).Order("messages.id desc").Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// GetMessagesByApplicationSince returns limited messages from an application.
// If since is 0 it will be ignored.
func (d *GormDatabase) GetMessagesByApplicationSince(appID uint, limit int, since uint) ([]*model.Message, error) {
	var messages []*model.Message
	db := d.DB.Where("application_id = ?", appID).Order("messages.id desc").Limit(limit)
	if since != 0 {
		db = db.Where("messages.id < ?", since)
	}
	err := db.Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// GetMessagesByUserWithFilter returns limited messages from a user with structured filtering.
// If since is 0 it will be ignored. If filter is nil, behaves like GetMessagesByUserSince.
func (d *GormDatabase) GetMessagesByUserWithFilter(userID uint, limit int, since uint, filter *model.MessageFilter) ([]*model.Message, error) {
	var messages []*model.Message
	db := d.DB.Joins("JOIN applications ON applications.user_id = ?", userID).
		Where("messages.application_id = applications.id").Order("messages.id desc").Limit(limit)
	if since != 0 {
		db = db.Where("messages.id < ?", since)
	}
	db = applyMessageFilters(db, filter)
	err := db.Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// GetMessagesByApplicationWithFilter returns limited messages from an application with structured filtering.
// If since is 0 it will be ignored. If filter is nil, behaves like GetMessagesByApplicationSince.
func (d *GormDatabase) GetMessagesByApplicationWithFilter(appID uint, limit int, since uint, filter *model.MessageFilter) ([]*model.Message, error) {
	var messages []*model.Message
	db := d.DB.Where("application_id = ?", appID).Order("messages.id desc").Limit(limit)
	if since != 0 {
		db = db.Where("messages.id < ?", since)
	}
	db = applyMessageFilters(db, filter)
	err := db.Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
}

// applyMessageFilters chains WHERE clauses from a MessageFilter onto a GORM query.
func applyMessageFilters(db *gorm.DB, filter *model.MessageFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	if filter.ApplicationID != nil {
		db = db.Where("messages.application_id = ?", *filter.ApplicationID)
	}
	if filter.Priority != nil {
		db = db.Where("messages.priority = ?", *filter.Priority)
	}
	if filter.PriorityMin != nil {
		db = db.Where("messages.priority >= ?", *filter.PriorityMin)
	}
	if filter.PriorityMax != nil {
		db = db.Where("messages.priority <= ?", *filter.PriorityMax)
	}
	if filter.Before != nil {
		db = db.Where("messages.date < ?", *filter.Before)
	}
	if filter.After != nil {
		db = db.Where("messages.date > ?", *filter.After)
	}
	return db
}

// DeleteMessageByID deletes a message by its id.
func (d *GormDatabase) DeleteMessageByID(id uint) error {
	return d.DB.Where("id = ?", id).Delete(&model.Message{}).Error
}

// DeleteMessagesByApplication deletes all messages from an application.
func (d *GormDatabase) DeleteMessagesByApplication(applicationID uint) error {
	return d.DB.Where("application_id = ?", applicationID).Delete(&model.Message{}).Error
}

// DeleteMessagesByUser deletes all messages from a user.
func (d *GormDatabase) DeleteMessagesByUser(userID uint) error {
	app, _ := d.GetApplicationsByUser(userID)
	for _, app := range app {
		d.DeleteMessagesByApplication(app.ID)
	}
	return nil
}
