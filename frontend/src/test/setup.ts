// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

// Global test setup: initialize i18n so hooks like useTranslation work in tests.
// Mirrors the side-effecting import that main.tsx performs at app boot.
import '@/i18n'
