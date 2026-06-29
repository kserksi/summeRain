// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

export class ApiError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}
