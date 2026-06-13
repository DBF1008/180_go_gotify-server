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

// applyMessageFilter adds the ordering, pagination cursor, content filter and
// limit clauses shared by the filtered message queries. The caller is
// responsible for the user/application scoping. AppIDs is not applied here; it
// only makes sense for the user-wide query, which adds it itself.
func applyMessageFilter(db *gorm.DB, filter *model.MessageFilter, limit int, since uint) *gorm.DB {
	db = db.Order("messages.id desc")
	if since != 0 {
		db = db.Where("messages.id < ?", since)
	}
	if filter != nil {
		if filter.PriorityFrom != nil {
			db = db.Where("messages.priority >= ?", *filter.PriorityFrom)
		}
		if filter.PriorityUntil != nil {
			db = db.Where("messages.priority <= ?", *filter.PriorityUntil)
		}
		// The date column is compared as stored. Clients should send RFC3339
		// timestamps in UTC: on sqlite the column is text, so a comparison
		// against a differing timezone offset would be lexical, not temporal.
		if filter.DateFrom != nil {
			db = db.Where("messages.date >= ?", *filter.DateFrom)
		}
		if filter.DateUntil != nil {
			db = db.Where("messages.date <= ?", *filter.DateUntil)
		}
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	return db
}

// GetMessagesByUserSince returns limited messages from a user.
// If since is 0 it will be ignored.
func (d *GormDatabase) GetMessagesByUserSince(userID uint, limit int, since uint) ([]*model.Message, error) {
	return d.GetMessagesByUserWithFilter(userID, nil, limit, since)
}

// GetMessagesByUserWithFilter returns limited messages from a user matching the
// given filter. A nil filter applies no content criteria. If since is 0 it will
// be ignored.
func (d *GormDatabase) GetMessagesByUserWithFilter(userID uint, filter *model.MessageFilter, limit int, since uint) ([]*model.Message, error) {
	var messages []*model.Message
	query := d.DB.Joins("JOIN applications ON applications.user_id = ?", userID).
		Where("messages.application_id = applications.id")
	if filter != nil && len(filter.AppIDs) > 0 {
		query = query.Where("messages.application_id IN ?", filter.AppIDs)
	}
	err := applyMessageFilter(query, filter, limit, since).Find(&messages).Error
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
	return d.GetMessagesByApplicationWithFilter(appID, nil, limit, since)
}

// GetMessagesByApplicationWithFilter returns limited messages from an
// application matching the given filter. A nil filter applies no content
// criteria. If since is 0 it will be ignored. The filter's AppIDs are ignored
// because the application is already fixed by appID.
func (d *GormDatabase) GetMessagesByApplicationWithFilter(appID uint, filter *model.MessageFilter, limit int, since uint) ([]*model.Message, error) {
	var messages []*model.Message
	query := d.DB.Where("application_id = ?", appID)
	err := applyMessageFilter(query, filter, limit, since).Find(&messages).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return messages, err
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
