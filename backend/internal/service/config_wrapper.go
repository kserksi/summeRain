// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import "github.com/summerain/image-gallery/internal/repository"

type ConfigRepoWrapper struct {
	Repo *repository.SystemConfigRepo
}

func (w *ConfigRepoWrapper) FindByKey(key string) (*rdbConfigValue, error) {
	cfg, err := w.Repo.FindByKey(key)
	if err != nil {
		return nil, err
	}
	return &rdbConfigValue{Value: cfg.ConfigValue}, nil
}
