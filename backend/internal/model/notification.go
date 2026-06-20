package model

import (
	"time"

	"gorm.io/gorm"
)

type Notification struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64         `gorm:"index;not null" json:"user_id"`
	Type      string         `gorm:"size:50;not null" json:"type"`
	Title     string         `gorm:"size:200;not null" json:"title"`
	Message   string         `gorm:"type:text" json:"message"`
	IsRead    bool           `gorm:"default:false;not null" json:"is_read"`
	Metadata  string         `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.Metadata == "" {
		n.Metadata = "{}"
	}
	return nil
}
