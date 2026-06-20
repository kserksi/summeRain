package repository

import (
	"github.com/summerain/image-gallery/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SystemConfigRepo struct {
	db *gorm.DB
}

func NewSystemConfigRepo(db *gorm.DB) *SystemConfigRepo {
	return &SystemConfigRepo{db: db}
}

func (r *SystemConfigRepo) FindAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := r.db.Find(&configs).Error
	return configs, err
}

func (r *SystemConfigRepo) FindByKey(key string) (*model.SystemConfig, error) {
	var cfg model.SystemConfig
	err := r.db.Where("config_key = ?", key).First(&cfg).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *SystemConfigRepo) BatchUpdate(updates []model.SystemConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, u := range updates {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "config_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"config_value"}),
			}).Create(&u).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
