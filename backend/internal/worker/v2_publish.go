// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

func (m *Manager) runV2Publish(ctx context.Context, drain <-chan struct{}, index int) {
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s:%d:publish:%d", hostname, os.Getpid(), index)
	for {
		if ctx.Err() != nil || workerDrainRequested(drain) {
			return
		}
		processed, err := m.V2.ProcessNextPublishJob(ctx, workerID)
		if err != nil && ctx.Err() == nil {
			log.Printf("[v2_publish] worker=%s error=%v", workerID, err)
		}
		if processed {
			continue
		}
		timer := time.NewTimer(m.Config.ImageV2.JobPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-drain:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}
