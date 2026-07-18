// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import "testing"

func TestV1R2UploadIntentLeaseOutlastsHardUploadDeadline(t *testing.T) {
	if v1R2UploadIntentLease <= v1R2UploadTimeout+v1R2UploadLeaseSafety {
		t.Fatalf("intent lease %s must outlast upload timeout %s plus safety margin %s",
			v1R2UploadIntentLease, v1R2UploadTimeout, v1R2UploadLeaseSafety)
	}
}
