package repository

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/summerain/image-gallery/internal/model"
	"gorm.io/gorm"
)

type ImageRepo struct {
	db *gorm.DB
}

func NewImageRepo(db *gorm.DB) *ImageRepo {
	return &ImageRepo{db: db}
}

func (r *ImageRepo) Create(image *model.Image) error {
	return r.db.Create(image).Error
}

func (r *ImageRepo) FindByID(id uint64) (*model.Image, error) {
	var image model.Image
	if err := r.db.First(&image, id).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

func (r *ImageRepo) FindByUniqueLink(link string) (*model.Image, error) {
	var image model.Image
	if err := r.db.Where("unique_link = ?", link).First(&image).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

func (r *ImageRepo) FindByUserID(userID uint64, cursor string, limit int, sort string, visibility string, search string) ([]*model.Image, string, error) {
	query := r.db.Where("user_id = ?", userID)

	if visibility != "" {
		query = query.Where("visibility = ?", visibility)
	}
	if search != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search)
		query = query.Where("filename LIKE ? ESCAPE '\\\\' OR description LIKE ? ESCAPE '\\\\'", "%"+escaped+"%", "%"+escaped+"%")
	}

	if cursor != "" {
		cursorTime, cursorID, err := decodeCursor(cursor)
		if err == nil {
			query = query.Where("(created_at < ?) OR (created_at = ? AND id < ?)", cursorTime, cursorTime, cursorID)
		}
	}

	orderClause := "created_at DESC, id DESC"
	switch sort {
	case "created_at":
		orderClause = "created_at ASC, id ASC"
	case "-views":
		orderClause = "view_count DESC, id DESC"
	case "views":
		orderClause = "view_count ASC, id ASC"
	}

	var images []*model.Image
	if err := query.Order(orderClause).Limit(limit + 1).Find(&images).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(images) > limit {
		last := images[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		images = images[:limit]
	}

	return images, nextCursor, nil
}

func (r *ImageRepo) Delete(id uint64) error {
	return r.db.Delete(&model.Image{}, id).Error
}

func (r *ImageRepo) UpdateVisibility(id uint64, visibility string) error {
	return r.db.Model(&model.Image{}).Where("id = ?", id).Update("visibility", visibility).Error
}

func (r *ImageRepo) IncrementViewCount(id uint64) error {
	return r.db.Model(&model.Image{}).Where("id = ?", id).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

type ImageFileRepo struct {
	db *gorm.DB
}

func NewImageFileRepo(db *gorm.DB) *ImageFileRepo {
	return &ImageFileRepo{db: db}
}

func (r *ImageFileRepo) Create(file *model.ImageFile) error {
	return r.db.Create(file).Error
}

func (r *ImageFileRepo) FindByHash(hash string) (*model.ImageFile, error) {
	var file model.ImageFile
	if err := r.db.Where("file_hash = ?", hash).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (r *ImageFileRepo) FindByID(id uint64) (*model.ImageFile, error) {
	var file model.ImageFile
	if err := r.db.First(&file, id).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (r *ImageFileRepo) Delete(id uint64) error {
	return r.db.Delete(&model.ImageFile{}, id).Error
}

func encodeCursor(t time.Time, id uint64) string {
	raw := fmt.Sprintf("%s|%d", t.Format(time.RFC3339Nano), id)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, uint64, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	var id uint64
	_, err = fmt.Sscanf(parts[1], "%d", &id)
	if err != nil {
		return time.Time{}, 0, err
	}
	return t, id, nil
}

func (r *ImageRepo) FindAll(page, pageSize int) ([]*model.Image, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	r.db.Model(&model.Image{}).Count(&total)

	var images []*model.Image
	if err := r.db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&images).Error; err != nil {
		return nil, 0, err
	}
	return images, total, nil
}

func (r *ImageRepo) DeleteByUserID(userID uint64) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.Image{}).Error
}

func (r *ImageRepo) FindOriginalPathsByUserID(userID uint64) ([]*model.Image, error) {
	var images []*model.Image
	if err := r.db.Where("user_id = ?", userID).Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}
